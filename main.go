package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ditto-assistant/backend/cfg/secr"
	apiv1 "github.com/ditto-assistant/backend/pkg/api/v1"
	apiv2 "github.com/ditto-assistant/backend/pkg/api/v2"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/middleware"
	"github.com/ditto-assistant/backend/pkg/services/db"
	"github.com/ditto-assistant/backend/pkg/services/db/users"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/cerebras"
	"github.com/ditto-assistant/backend/pkg/services/llm/claude"
	"github.com/ditto-assistant/backend/pkg/services/llm/gemini"
	"github.com/ditto-assistant/backend/pkg/services/llm/googai"
	"github.com/ditto-assistant/backend/pkg/services/llm/llama"
	"github.com/ditto-assistant/backend/pkg/services/llm/mistral"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai/dalle"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai/gpt"
	"github.com/ditto-assistant/backend/pkg/services/search"
	"github.com/ditto-assistant/backend/pkg/services/search/brave"
	"github.com/ditto-assistant/backend/pkg/services/search/google"
	"github.com/ditto-assistant/backend/pkg/services/stripe"
	"github.com/ditto-assistant/backend/pkg/web"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/ditto-assistant/backend/types/ty"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var shutdownWG sync.WaitGroup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT)
	sdCtx := ty.ShutdownContext{
		Background:       bgCtx,
		WaitGroup:        &shutdownWG,
		ShutdownDuration: 30 * time.Second,
	}
	coreSvc, err := core.NewClient(bgCtx)
	if err != nil {
		log.Fatalf("failed to set up Firebase auth: %v", err)
	}
	if err := db.Setup(bgCtx, &shutdownWG, db.ModeCloud); err != nil {
		log.Fatalf("failed to initialize database: %s", err)
	}
	googaiClient, err := googai.NewClient(bgCtx)
	if err != nil {
		log.Fatalf("failed to initialize Google AI client: %v", err)
	}

	mux := http.NewServeMux()
	searchClient := search.NewClient(
		search.WithService(brave.NewService(sdCtx, coreSvc.Secr)),
		search.WithService(google.NewService(sdCtx, coreSvc.Secr)),
	)
	dalleClient := dalle.NewClient(secr.OPENAI_DALLE_API_KEY.String(), llm.HttpClient)
	v1Client := apiv1.NewService(sdCtx, coreSvc, apiv1.ServiceClients{
		SearchClient: searchClient,
		S3:           coreSvc.FileStorage.S3,
		Dalle:        dalleClient,
	})
	stripeClient := stripe.NewClient(coreSvc.Secr, coreSvc.Auth)
	mux.HandleFunc("GET /v1/balance", v1Client.Balance)
	mux.HandleFunc("POST /v1/create-upload-url", v1Client.CreateUploadURL)
	mux.HandleFunc("POST /v1/google-search", v1Client.WebSearch)
	mux.HandleFunc("POST /v1/generate-image", v1Client.GenerateImage)
	mux.HandleFunc("POST /v1/presign-url", v1Client.PresignURL)
	mux.HandleFunc("POST /v1/get-memories", v1Client.GetMemories)
	mux.HandleFunc("POST /v1/feedback", v1Client.Feedback)
	mux.HandleFunc("POST /v1/stripe/checkout-session", stripeClient.CreateCheckoutSession)
	mux.HandleFunc("POST /v1/stripe/webhook", stripeClient.HandleWebhook)

	v2Client := apiv2.NewService(coreSvc)
	v2Client.Routes(mux)
	cerebrasClient := cerebras.NewService(&sdCtx, coreSvc.Secr)

	webClient := web.NewClient(coreSvc)
	webClient.Routes(mux)

	// - MARK: prompt
	mux.HandleFunc("POST /v1/prompt", func(w http.ResponseWriter, r *http.Request) {
		tok, err := coreSvc.Auth.VerifyToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.PromptV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = tok.Check(bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		user := users.User{UID: bod.UserID}
		ctx := r.Context()
		if err := user.GetByUID(ctx, db.D); err != nil {
			slog.Error("failed to get user", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		slog := slog.With("action", "prompt", "userID", bod.UserID, "model", bod.Model, "email", user.Email.String)
		// llama32 is free
		if user.Balance <= 0 && bod.Model != llm.ModelLlama32 {
			slog.Error("user balance is 0", "balance", user.Balance)
			http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
			return
		}

		var rsp llm.StreamResponse
		switch bod.Model {
		case
			llm.ModelClaude3Haiku, llm.ModelClaude3Haiku_20240307,
			llm.ModelClaude35Sonnet, llm.ModelClaude35Sonnet_20240620,
			llm.ModelClaude35SonnetV2, llm.ModelClaude35SonnetV2_20241022,
			llm.ModelClaude35Haiku, llm.ModelClaude35Haiku_20241022:
			err = claude.Prompt(ctx, bod, &rsp)
			if err != nil {
				slog.Error("failed to prompt Claude", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case llm.ModelGemini15Flash:
			m := gemini.ModelGemini15Flash
			err = m.Prompt(ctx, bod, &rsp)
			if err != nil {
				slog.Error("failed to prompt "+m.PrettyStr(), "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case llm.ModelGemini15Pro:
			m := gemini.ModelGemini15Pro
			err = m.Prompt(ctx, bod, &rsp)
			if err != nil {
				slog.Error("failed to prompt "+m.PrettyStr(), "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case llm.ModelMistralNemo, llm.ModelMistralLarge:
			err = mistral.Prompt(ctx, bod, &rsp)
			if err != nil {
				slog.Error("failed to prompt "+bod.Model.String(), "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case llm.ModelLlama32:
			m := llama.ModelLlama32
			err = m.Prompt(ctx, bod, &rsp)
			if err != nil {
				slog.Error("failed to prompt "+m.PrettyStr(), "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case
			llm.ModelO1Mini, llm.ModelO1Mini_20240912,
			llm.ModelO1Preview, llm.ModelO1Preview_20240912,
			llm.ModelGPT4oMini, llm.ModelGPT4oMini_20240718,
			llm.ModelGPT4o, llm.ModelGPT4o_1120:
			err = gpt.Prompt(ctx, bod, &rsp)
			if err != nil {
				slog.Error("failed to prompt "+bod.Model.String(), "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case llm.ModelCerebrasLlama8B, llm.ModelCerebrasLlama70B:
			err = cerebrasClient.Prompt(ctx, bod, &rsp)
			if err != nil {
				slog.Error("failed to prompt "+bod.Model.String(), "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			slog.Info("unsupported model", "model", bod.Model)
			http.Error(w, fmt.Sprintf("unsupported model: %s", bod.Model), http.StatusBadRequest)
			return
		}
		for token := range rsp.Text {
			if token.Err != nil {
				slog.Error("failed to stream token", "error", token.Err)
				http.Error(w, token.Err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, token.Ok)
		}

		sdCtx.Run(func(ctx context.Context) {
			slog.Debug("receipt", "input_tokens", rsp.InputTokens, "output_tokens", rsp.OutputTokens)
			receipt := db.Receipt{
				UserID:       user.ID,
				InputTokens:  int64(rsp.InputTokens),
				OutputTokens: int64(rsp.OutputTokens),
				ServiceName:  bod.Model,
			}
			if err := receipt.Insert(ctx); err != nil {
				slog.Error("failed to insert receipt", "error", err)
			}
		})
	})

	// - MARK: embed
	mux.HandleFunc("POST /v1/embed", func(w http.ResponseWriter, r *http.Request) {
		tok, err := coreSvc.Auth.VerifyToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.EmbedV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = tok.Check(bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		user := users.User{UID: bod.UserID}
		ctx := r.Context()
		if err := user.GetByUID(ctx, db.D); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if bod.Model == "" {
			bod.Model = llm.ModelTextEmbedding004
		}
		slog := slog.With("action", "embed", "userID", bod.UserID, "model", bod.Model, "email", user.Email.String)

		var embedding llm.Embedding
		if bod.Model == llm.ModelTextEmbedding3Small {
			embedding, err = openai.GenerateEmbedding(ctx, bod.Text, bod.Model)
		} else {
			embeddings, err := googaiClient.Embed(ctx, googai.EmbedRequest{
				Documents: []string{bod.Text},
				Model:     bod.Model,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			embedding = embeddings[0]
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedding)

		sdCtx.Run(func(ctx context.Context) {
			tokens := llm.EstimateTokens(bod.Text)
			slog.Debug("receipt", "input_tokens", tokens)
			receipt := db.Receipt{
				UserID:      user.ID,
				TotalTokens: int64(tokens),
				ServiceName: bod.Model,
			}
			if err := receipt.Insert(ctx); err != nil {
				slog.Error("failed to insert receipt", "error", err)
			}
		})
	})

	// - MARK: search-examples
	mux.HandleFunc("POST /v1/search-examples", func(w http.ResponseWriter, r *http.Request) {
		tok, err := coreSvc.Auth.VerifyToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.SearchExamplesV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = tok.Check(bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
		}
		if bod.K == 0 {
			bod.K = 5
		}
		ctx := r.Context()
		examples, err := db.SearchExamples(ctx, bod.Embedding, db.WithK(bod.K))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Format response
		w.Write([]byte{'\n'})
		for i, example := range examples {
			fmt.Fprintf(w, "Example %d\n", i+1)
			fmt.Fprintf(w, "User's Prompt: %s\nDitto:\n%s\n\n", example.Prompt, example.Response)
		}
	})

	corsMiddleware := middleware.NewCors()
	handler := corsMiddleware.Handler(mux)
	server := &http.Server{
		Addr:    ":3400",
		Handler: handler,
	}

	go func() {
		select {
		case sig := <-sigChan:
			slog.Info("Received SIG; shutting down", "signal", sig)
			server.Shutdown(bgCtx)
		}
	}()

	slog.Debug("Starting server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	cancel()
	shutdownWG.Wait()
	os.Exit(0)
}
