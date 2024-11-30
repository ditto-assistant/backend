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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/api/accounts"
	"github.com/ditto-assistant/backend/pkg/api/stripe"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/db/users"
	"github.com/ditto-assistant/backend/pkg/fbase"
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
	"github.com/ditto-assistant/backend/pkg/search/brave"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/firebase/genkit/go/plugins/vertexai"
	"github.com/omniaura/mapcache"
	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
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
	if err := secr.Setup(bgCtx); err != nil {
		log.Fatalf("failed to initialize secrets: %s", err)
	}
	if err := db.Setup(bgCtx, &shutdownWG, db.ModeCloud); err != nil {
		log.Fatalf("failed to initialize database: %s", err)
	}
	stripe.Setup()
	auth, err := fbase.NewAuth(bgCtx)
	if err != nil {
		log.Fatalf("failed to set up Firebase auth: %v", err)
	}
	mux := http.NewServeMux()
	bucket := aws.String(envs.DITTO_CONTENT_BUCKET)
	s3Config := &aws.Config{
		Credentials: credentials.NewStaticCredentials(envs.BACKBLAZE_KEY_ID, secr.BACKBLAZE_API_KEY.String(), ""),
		Region:      aws.String(envs.DITTO_CONTENT_REGION),
		Endpoint:    aws.String(envs.DITTO_CONTENT_ENDPOINT),
	}
	mySession, err := session.NewSession(s3Config)
	if err != nil {
		log.Fatalf("failed to create session: %v", err)
	}
	s3Client := s3.New(mySession)

	// - MARK: prompt
	mux.HandleFunc("POST /v1/prompt", func(w http.ResponseWriter, r *http.Request) {
		tok, err := auth.VerifyToken(r)
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
		if user.Balance <= 0 {
			slog.Error("user balance is 0", "balance", user.Balance)
			http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
			return
		}

		var rsp llm.StreamResponse
		switch bod.Model {
		case llm.ModelClaude35Sonnet:
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
			llm.ModelO1Mini, llm.ModelO1Mini20240912,
			llm.ModelO1Preview, llm.ModelO1Preview20240912,
			llm.ModelGPT4oMini, llm.ModelGPT4oMini20240718,
			llm.ModelGPT4o, llm.ModelGPT4o1120:
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
		tok, err := auth.VerifyToken(r)
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
		if user.Balance <= 0 {
			http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
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

	customSearch, err := customsearch.NewService(bgCtx, option.WithAPIKey(secr.SEARCH_API_KEY.String()))
	if err != nil {
		log.Fatalf("failed to initialize custom search: %s", err)
	}
	// - MARK: google-search
	mux.HandleFunc("POST /v1/google-search", func(w http.ResponseWriter, r *http.Request) {
		tok, err := auth.VerifyToken(r)
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
		err = tok.Check(bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if bod.NumResults == 0 {
			bod.NumResults = 5
		}
		user := users.User{UID: bod.UserID}
		ctx := r.Context()
		if err := user.Get(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if user.Balance <= 0 {
			http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
			return
		}
		search, err := brave.Search(r.Context(), bod.Query, bod.NumResults)
		if err == nil {
			search.Text(w)

			shutdownWG.Add(1)
			go func() {
				defer shutdownWG.Done()
				ctx, cancel := context.WithTimeout(bgCtx, 15*time.Second)
				defer cancel()
				receipt := db.Receipt{
					UserID:      user.ID,
					NumSearches: 1,
					ServiceName: llm.SearchEngineBrave,
				}
				if err := receipt.Insert(ctx); err != nil {
					slog.Error("failed to insert receipt", "error", err)
				}
			}()
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

		shutdownWG.Add(1)
		go func() {
			defer shutdownWG.Done()
			ctx, cancel := context.WithTimeout(bgCtx, 15*time.Second)
			defer cancel()
			receipt := db.Receipt{
				UserID:      user.ID,
				NumSearches: 1,
				ServiceName: llm.SearchEngineGoogle,
			}
			if err := receipt.Insert(ctx); err != nil {
				slog.Error("failed to insert receipt", "error", err)
			}
		}()
	})

	// - MARK: presign-url
	const presignTTL = 24 * time.Hour
	urlCache, _ := mapcache.New[string, string](
		mapcache.WithTTL(presignTTL/2),
		mapcache.WithCleanup(bgCtx, presignTTL),
	)
	mux.HandleFunc("POST /v1/presign-url", func(w http.ResponseWriter, r *http.Request) {
		tok, err := auth.VerifyToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var bod rq.PresignedURLV1
		if err := json.Unmarshal(body, &bod); err != nil {
			slog.Error("failed to decode request body", "error", err, "body", string(body), "path", r.URL.Path)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = tok.Check(bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		url, err := urlCache.Get(bod.URL, func() (string, error) {
			urlParts := strings.Split(bod.URL, "?")
			if len(urlParts) == 0 {
				return "", fmt.Errorf("failed to get filename from URL: %s", bod.URL)
			}
			if bod.Folder == "" {
				bod.Folder = "generated-images"
			}
			// Clean filename
			filename := strings.TrimPrefix(urlParts[0], envs.DITTO_CONTENT_PREFIX)
			filename = strings.TrimPrefix(filename, envs.DALL_E_PREFIX)
			filename = strings.TrimPrefix(filename, bod.UserID+"/")
			filename = strings.TrimPrefix(filename, bod.Folder+"/")
			key := fmt.Sprintf("%s/%s/%s", bod.UserID, bod.Folder, filename)
			objReq, _ := s3Client.GetObjectRequest(&s3.GetObjectInput{
				Bucket: bucket,
				Key:    aws.String(key),
			})
			url, err := objReq.Presign(presignTTL)
			if err != nil {
				return "", fmt.Errorf("failed to generate presigned URL: %s", err)
			}
			return url, nil
		})
		if err != nil {
			slog.Error("failed to generate presigned URL", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, url)
	})

	// - MARK: create-upload-url
	mux.HandleFunc("POST /v1/create-upload-url", func(w http.ResponseWriter, r *http.Request) {
		tok, err := auth.VerifyToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.CreateUploadURLV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = tok.Check(bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		key := fmt.Sprintf("%s/uploads/%d", bod.UserID, time.Now().UnixNano())
		req, _ := s3Client.PutObjectRequest(&s3.PutObjectInput{
			Bucket: bucket,
			Key:    aws.String(key),
		})
		url, err := req.Presign(15 * time.Minute)
		slog.Debug("created upload URL", "url", url)
		if err != nil {
			slog.Error("failed to generate upload URL", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, url)
	})

	dalleClient := dalle.NewClient(secr.OPENAI_DALLE_API_KEY.String(), llm.HttpClient)
	// - MARK: generate-image
	mux.HandleFunc("POST /v1/generate-image", func(w http.ResponseWriter, r *http.Request) {
		tok, err := auth.VerifyToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.GenerateImageV1
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
		slog := slog.With("user_id", bod.UserID, "model", bod.Model)
		if err := user.Get(ctx); err != nil {
			slog.Error("failed to get user", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if user.Balance <= 0 {
			http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
			return
		}
		url, err := dalleClient.Prompt(ctx, &bod)
		if err != nil {
			slog.Error("failed to generate image", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, url)
		slog.Debug("generated image", "url", url)

		shutdownWG.Add(1)
		go func() {
			defer shutdownWG.Done()
			ctx, cancel := context.WithTimeout(bgCtx, 15*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				slog.Error("failed to create image request", "error", err)
				return
			}
			imgResp, err := http.DefaultClient.Do(req)
			if err != nil {
				slog.Error("failed to download image", "error", err)
				return
			}
			defer imgResp.Body.Close()
			imgData, err := io.ReadAll(imgResp.Body)
			if err != nil {
				slog.Error("failed to read image data", "error", err)
				return
			}
			urlParts := strings.Split(url, "?")
			if len(urlParts) == 0 {
				slog.Error("failed to get filename from URL", "url", url)
				return
			}
			filename := strings.TrimPrefix(urlParts[0], envs.DALL_E_PREFIX)
			key := fmt.Sprintf("%s/generated-images/%s", bod.UserID, filename)
			put, err := s3Client.PutObject(&s3.PutObjectInput{
				Bucket: bucket,
				Key:    aws.String(key),
				Body:   bytes.NewReader(imgData),
			})
			if err != nil {
				slog.Error("failed to copy to S3", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			slog.Debug("uploaded image to S3", "key", put.String())
			receipt := db.Receipt{
				UserID:      user.ID,
				NumImages:   1,
				ServiceName: bod.Model,
			}
			if err := receipt.Insert(ctx); err != nil {
				slog.Error("failed to insert receipt", "error", err)
			}
		}()
	})

	// - MARK: search-examples
	mux.HandleFunc("POST /v1/search-examples", func(w http.ResponseWriter, r *http.Request) {
		tok, err := auth.VerifyToken(r)
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

	mux.HandleFunc("GET /v1/balance", accounts.GetBalanceV1)

	// - MARK: stripe
	mux.HandleFunc("POST /v1/stripe/checkout-session", stripe.CreateCheckoutSession)
	mux.HandleFunc("POST /v1/stripe/webhook", stripe.HandleWebhook)

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
