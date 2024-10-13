package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/img"
	"github.com/ditto-assistant/backend/pkg/rq"
	"github.com/ditto-assistant/backend/pkg/search/brave"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/firebase"
	"github.com/firebase/genkit/go/plugins/vertexai"
	"github.com/rs/cors"
	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var shutdown sync.WaitGroup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT)

	if err := vertexai.Init(ctx, &vertexai.Config{
		ProjectID: "ditto-app-dev",
		Location:  "us-central1",
	}); err != nil {
		log.Fatal(err)
	}
	if err := secr.Setup(ctx); err != nil {
		log.Fatalf("failed to initialize secrets: %s", err)
	}
	if err := db.Setup(ctx, &shutdown); err != nil {
		log.Fatalf("failed to initialize database: %s", err)
	}

	firebaseAuth, err := firebase.NewAuth(ctx, func(authContext genkit.AuthContext, input any) error {
		in, ok := input.(rq.HasUserID) // The type must match the input type of the flow.
		if !ok {
			return fmt.Errorf("request body type is incorrect: %T", input)
		}
		uidIn := in.GetUserID()
		if len(authContext) == 0 {
			return fmt.Errorf("authContext is empty; input uid: %s", uidIn)
		}
		uidAuth, ok := authContext["uid"]
		if !ok {
			return fmt.Errorf("authContext missing uid: %v", authContext)
		}
		if uidAuth, ok := uidAuth.(string); !ok {
			return fmt.Errorf("authContext uid is not a string: %v", uidAuth)
		} else if uidAuth != uidIn {
			return fmt.Errorf("user ID does not match: authContext uid: %v != input uid: %s", uidAuth, uidIn)
		}
		return nil
	}, true)
	if err != nil {
		log.Fatalf("failed to set up Firebase auth: %v", err)
	}

	genkit.DefineStreamingFlow("v1/prompt",
		func(ctx context.Context, in rq.PromptV1, callback func(context.Context, string) error) (string, error) {
			if in.Model != "" {
				if !vertexai.IsDefinedModel(in.Model) {
					return "", fmt.Errorf("promptFlow: model not found: %s", in.Model)
				}
			} else {
				in.Model = "gemini-1.5-pro"
			}
			m := vertexai.Model(in.Model)
			messages := []*ai.Message{
				ai.NewSystemTextMessage(in.SystemPrompt),
				ai.NewUserTextMessage(in.UserPrompt),
			}
			if in.ImageURL != "" {
				imgPart, err := img.NewPart(ctx, in.ImageURL)
				if err != nil {
					return "", err
				}
				messages = append(messages, ai.NewUserMessage(imgPart))
			}
			cfg := &ai.GenerationCommonConfig{Temperature: 0.5}
			resp, err := m.Generate(ctx,
				ai.NewGenerateRequest(cfg, messages...),
				func(ctx context.Context, grc *ai.GenerateResponseChunk) error {
					if callback == nil {
						return nil
					}
					return callback(ctx, grc.Text())
				},
			)
			if err != nil {
				return "", err
			}
			return resp.Text(), nil
		},
		genkit.WithFlowAuth(firebaseAuth),
	)

	go func() {
		err := genkit.Init(ctx, &genkit.Options{FlowAddr: "-"})
		if err != nil {
			log.Fatalf("failed to initialize genkit: %v", err)
		}
	}()

	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:3000", "https://assistant.heyditto.ai"}, // Allow all origins
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"}, // Allow all headers
		MaxAge:         86400,         // 24 hours
	})
	mux := genkit.NewFlowServeMux(nil)

	mux.HandleFunc("POST /v1/embed", func(w http.ResponseWriter, r *http.Request) {
		ctx, err := firebaseAuth.ProvideAuthContext(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.EmbedV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = firebaseAuth.CheckAuthPolicy(ctx, bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return

		}
		if bod.Model == "text-embedding-3-small" {
			// OpenAI Embeddings
			type RequestEmbeddingOpenAI struct {
				Input          string `json:"input"`
				Model          string `json:"model"`
				EncodingFormat string `json:"encoding_format"`
			}
			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(RequestEmbeddingOpenAI{
				Input:          bod.Text,
				Model:          bod.Model,
				EncodingFormat: "float",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", &buf)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+secr.OPENAI_EMBEDDINGS_API_KEY)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				http.Error(w, fmt.Sprintf("failed to generate embedding: %s", resp.Status), resp.StatusCode)
				return
			}
			var respBody struct {
				Data []struct {
					Embedding []float32 `json:"embedding"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(respBody.Data) == 0 {
				http.Error(w, "no embeddings returned", http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(respBody.Data[0].Embedding)
			return
		}

		// Google Vertex AI Embedders
		if bod.Model != "" {
			if !vertexai.IsDefinedEmbedder(bod.Model) {
				http.Error(w, fmt.Sprintf("embedFlow: model not found: %s", bod.Model), http.StatusBadRequest)
				return
			}
		} else {
			bod.Model = "text-embedding-004"
		}
		embedder := vertexai.Embedder(bod.Model)
		embeddings, err := embedder.Embed(ctx, &ai.EmbedRequest{
			Documents: []*ai.Document{
				{
					Content: []*ai.Part{
						ai.NewTextPart(bod.Text),
					},
				},
			},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(embeddings.Embeddings) == 0 {
			http.Error(w, "no embeddings returned", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(embeddings.Embeddings[0].Embedding)
	})

	customSearch, err := customsearch.NewService(ctx, option.WithAPIKey(secr.SEARCH_API_KEY))
	if err != nil {
		log.Fatalf("failed to initialize custom search: %s", err)
	}
	mux.HandleFunc("POST /v1/google-search", func(w http.ResponseWriter, r *http.Request) {
		ctx, err := firebaseAuth.ProvideAuthContext(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.SearchV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			if err == io.EOF {
				http.Error(w, "request body is empty", http.StatusBadRequest)
			} else {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return
		}
		err = firebaseAuth.CheckAuthPolicy(ctx, bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if bod.NumResults == 0 {
			bod.NumResults = 5
		}
		search, err := brave.Search(r.Context(), bod.Query, bod.NumResults)
		if err == nil {
			search.Text(w)
			return
		}
		slog.Error("failed to search with Brave, trying Google", "error", err)
		ser, err := customSearch.Cse.List().Do(
			googleapi.QueryParameter("q", bod.Query),
			googleapi.QueryParameter("num", strconv.Itoa(bod.NumResults)),
			googleapi.QueryParameter("cx", os.Getenv("SEARCH_ENGINE_ID")),
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte{'\n', '\n'})
		if len(ser.Items) == 0 {
			slog.Warn("no search results found", "query", bod.Query)
			fmt.Fprintln(w, "No results found")
			return
		}
		for i, item := range ser.Items {
			fmt.Fprintf(w,
				"%d. [%s](%s)\n\t- %s\n\n",
				i+1, item.Title, item.Link, item.Snippet,
			)
		}
	})

	dalleClient := &http.Client{
		Timeout: 20 * time.Second,
	}
	mux.HandleFunc("POST /v1/generate-image", func(w http.ResponseWriter, r *http.Request) {
		ctx, err := firebaseAuth.ProvideAuthContext(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.GenerateImageV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = firebaseAuth.CheckAuthPolicy(ctx, bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if bod.Model == "" {
			bod.Model = "dall-e-3"
		}
		type RequestImageOpenAI struct {
			Prompt string `json:"prompt"`
			Model  string `json:"model"`
			N      int    `json:"n"`
			Size   string `json:"size"`
		}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(RequestImageOpenAI{
			Prompt: bod.Prompt,
			Model:  bod.Model,
			N:      1,
			Size:   "1024x1024",
		}); err != nil {
			slog.Error("failed to encode image request", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/images/generations", &buf)
		if err != nil {
			slog.Error("failed to create image request", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+secr.OPENAI_DALLE_API_KEY)
		resp, err := dalleClient.Do(req)
		if err != nil {
			slog.Error("failed to send image request", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			slog.Error("failed to generate image", "status", resp.Status, "status code", resp.StatusCode)
			http.Error(w, fmt.Sprintf("failed to generate image: %s", resp.Status), resp.StatusCode)
			return
		}
		var respBody struct {
			Data []struct {
				URL string `json:"url"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			slog.Error("failed to decode image response", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(respBody.Data) == 0 {
			slog.Error("no image URL returned")
			http.Error(w, "no image URL returned", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, respBody.Data[0].URL)
		slog.Debug("generated image", "url", respBody.Data[0].URL)
	})

	mux.HandleFunc("POST /v1/search-examples", func(w http.ResponseWriter, r *http.Request) {
		ctx, err := firebaseAuth.ProvideAuthContext(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.SearchExamplesV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if bod.K == 0 {
			bod.K = 5
		}
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

	handler := corsMiddleware.Handler(mux)
	server := &http.Server{
		Addr:    ":3400",
		Handler: handler,
	}

	go func() {
		select {
		case sig := <-sigChan:
			slog.Info("Received SIG; shutting down", "signal", sig)
			server.Shutdown(ctx)
		}
	}()

	slog.Debug("Starting server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	cancel()
	shutdown.Wait()
	os.Exit(0)
}
