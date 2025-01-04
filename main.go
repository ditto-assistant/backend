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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/api/v1"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/db/users"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/pkg/llm/claude"
	"github.com/ditto-assistant/backend/pkg/llm/gemini"
	"github.com/ditto-assistant/backend/pkg/llm/googai"
	"github.com/ditto-assistant/backend/pkg/llm/llama"
	"github.com/ditto-assistant/backend/pkg/llm/mistral"
	"github.com/ditto-assistant/backend/pkg/llm/openai"
	"github.com/ditto-assistant/backend/pkg/llm/openai/dalle"
	"github.com/ditto-assistant/backend/pkg/llm/openai/gpt"
	"github.com/ditto-assistant/backend/pkg/middleware"
	"github.com/ditto-assistant/backend/pkg/search"
	"github.com/ditto-assistant/backend/pkg/search/brave"
	"github.com/ditto-assistant/backend/pkg/search/google"
	"github.com/ditto-assistant/backend/pkg/service"
	"github.com/ditto-assistant/backend/pkg/stripe"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/firebase/genkit/go/plugins/vertexai"
)

func main() {
	bgCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var shutdownWG sync.WaitGroup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT)
	if err := vertexai.Init(bgCtx, &vertexai.Config{
		ProjectID: "ditto-app-dev",
		Location:  "us-central1",
	}); err != nil {
		log.Fatal(err)
	}
	secrClient, err := secr.Setup(bgCtx)
	if err != nil {
		log.Fatalf("failed to initialize secrets: %s", err)
	}
	if err := db.Setup(bgCtx, &shutdownWG, db.ModeCloud); err != nil {
		log.Fatalf("failed to initialize database: %s", err)
	}
	firebaseApp, err := core.NewService(bgCtx)
	if err != nil {
		log.Fatalf("failed to set up Firebase auth: %v", err)
	}
	mux := http.NewServeMux()
	s3Config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(envs.BACKBLAZE_KEY_ID, secr.BACKBLAZE_API_KEY.String(), ""),
		Region:      aws.String(envs.DITTO_CONTENT_REGION),
		Endpoint:    aws.String(envs.DITTO_CONTENT_ENDPOINT),
	}
	mySession, err := session.NewSession(s3Config)
	if err != nil {
		log.Fatalf("failed to create session: %v", err)
	}
	svcCtx := service.Context{
		Background: bgCtx,
		ShutdownWG: &shutdownWG,
		Secr:       secrClient,
		App:        firebaseApp,
	}
	s3Client := s3.New(mySession)
	searchClient := search.NewClient(
		search.WithService(brave.NewService(svcCtx)),
		search.WithService(google.NewService(svcCtx)),
	)
	dalleClient := dalle.NewClient(secr.OPENAI_DALLE_API_KEY.String(), llm.HttpClient)
	v1Client := api.NewService(svcCtx, api.ServiceClients{
		SearchClient: searchClient,
		S3:           s3Client,
		Dalle:        dalleClient,
	})
	stripeClient := stripe.NewClient(svcCtx)
	mux.HandleFunc("GET /v1/balance", v1Client.Balance)
	mux.HandleFunc("POST /v1/create-upload-url", v1Client.CreateUploadURL)
	mux.HandleFunc("POST /v1/google-search", v1Client.WebSearch)
	mux.HandleFunc("POST /v1/generate-image", v1Client.GenerateImage)
	mux.HandleFunc("POST /v1/presign-url", v1Client.PresignURL)
	mux.HandleFunc("POST /v1/get-memories", v1Client.GetMemories)
	mux.HandleFunc("POST /v1/stripe/checkout-session", stripeClient.CreateCheckoutSession)
	mux.HandleFunc("POST /v1/stripe/webhook", stripeClient.HandleWebhook)

	// - MARK: prompt
	mux.HandleFunc("POST /v1/prompt", func(w http.ResponseWriter, r *http.Request) {
		tok, err := firebaseApp.VerifyToken(r)
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
		slog := slog.With("user_id", bod.UserID, "model", bod.Model)
		slog.Debug("Prompt Request")
		user := users.User{UID: bod.UserID}
		ctx := r.Context()
		if err := user.Get(ctx); err != nil {
			slog.Error("failed to get user", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		default:
			slog.Info("unsupported model", "model", bod.Model)
			http.Error(w, fmt.Sprintf("unsupported model: %s", bod.Model), http.StatusBadRequest)
			return
		}
		for token := range rsp.Text {
			if token.Err != nil {
				http.Error(w, token.Err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, token.Ok)
		}

		shutdownWG.Add(1)
		go func() {
			slog.Info("inserting receipt", "input_tokens", rsp.InputTokens, "output_tokens", rsp.OutputTokens)
			defer shutdownWG.Done()
			ctx, cancel := context.WithTimeout(bgCtx, 15*time.Second)
			defer cancel()
			receipt := db.Receipt{
				UserID:       user.ID,
				InputTokens:  int64(rsp.InputTokens),
				OutputTokens: int64(rsp.OutputTokens),
				ServiceName:  bod.Model,
			}
			if err := receipt.Insert(ctx); err != nil {
				slog.Error("failed to insert receipt", "error", err)
			}
		}()
	})

	// - MARK: embed
	mux.HandleFunc("POST /v1/embed", func(w http.ResponseWriter, r *http.Request) {
		tok, err := firebaseApp.VerifyToken(r)
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
		if err := user.Get(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var embedding llm.Embedding
		if bod.Model == llm.ModelTextEmbedding3Small {
			embedding, err = openai.GenerateEmbedding(ctx, bod.Text, bod.Model)
		} else {
			if bod.Model == "" {
				bod.Model = llm.ModelTextEmbedding004
			}
			embedding, err = googai.GenerateEmbedding(ctx, bod.Text, bod.Model)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedding)

		shutdownWG.Add(1)
		go func() {
			defer shutdownWG.Done()
			ctx, cancel := context.WithTimeout(bgCtx, 15*time.Second)
			defer cancel()
			receipt := db.Receipt{
				UserID:      user.ID,
				TotalTokens: int64(llm.EstimateTokens(bod.Text)),
				ServiceName: bod.Model,
			}
			if err := receipt.Insert(ctx); err != nil {
				slog.Error("failed to insert receipt", "error", err)
			}
		}()
	})

	// - MARK: search-examples
	mux.HandleFunc("POST /v1/search-examples", func(w http.ResponseWriter, r *http.Request) {
		tok, err := firebaseApp.VerifyToken(r)
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
