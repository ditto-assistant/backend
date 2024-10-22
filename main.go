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

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/pkg/llm/claude"
	"github.com/ditto-assistant/backend/pkg/llm/googai"
	"github.com/ditto-assistant/backend/pkg/llm/openai"
	"github.com/ditto-assistant/backend/pkg/numfmt"
	"github.com/ditto-assistant/backend/pkg/search/brave"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
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

	// genkit.DefineStreamingFlow("v1/prompt",
	// 	func(ctx context.Context, in rq.PromptV1, callback func(context.Context, string) error) (string, error) {
	// 		user, err := db.GetOrCreateUser(ctx, in.UserID)
	// 		if err != nil {
	// 			return "", fmt.Errorf("promptFlow: failed to get or create user: %w", err)
	// 		}
	// 		if user.Balance <= 0 {
	// 			return "", fmt.Errorf("balance is: %d", user.Balance)
	// 		}
	// 		if in.Model != "" {
	// 			if !vertexai.IsDefinedModel(in.Model.String()) {
	// 				return "", fmt.Errorf("promptFlow: model not found: %s", in.Model)
	// 			}
	// 		} else {
	// 			in.Model = llm.ModelGemini15Flash
	// 		}
	// 		m := vertexai.Model(in.Model.String())
	// 		messages := []*ai.Message{
	// 			ai.NewSystemTextMessage(in.SystemPrompt),
	// 			ai.NewUserTextMessage(in.UserPrompt),
	// 		}
	// 		if in.ImageURL != "" {
	// 			imgPart, err := img.NewPart(ctx, in.ImageURL)
	// 			if err != nil {
	// 				return "", err
	// 			}
	// 			messages = append(messages, ai.NewUserMessage(imgPart))
	// 		}
	// 		cfg := &ai.GenerationCommonConfig{Temperature: 0.5}
	// 		resp, err := m.Generate(ctx,
	// 			ai.NewGenerateRequest(cfg, messages...),
	// 			func(ctx context.Context, grc *ai.GenerateResponseChunk) error {
	// 				if callback == nil {
	// 					return nil
	// 				}
	// 				return callback(ctx, grc.Text())
	// 			},
	// 		)
	// 		if err != nil {
	// 			return "", err
	// 		}
	// 		textOut := resp.Text()
	// 		go func() {
	// 			inputTokens := llm.EstimateTokens(in.UserPrompt) + llm.EstimateTokens(in.SystemPrompt)
	// 			outputTokens := llm.EstimateTokens(textOut)
	// 			receipt := db.Receipt{
	// 				UserID:       user.ID,
	// 				InputTokens:  int64(inputTokens),
	// 				OutputTokens: int64(outputTokens),
	// 				ServiceName:  in.Model,
	// 			}
	// 			if err := receipt.Insert(ctx); err != nil {
	// 				slog.Error("failed to insert receipt", "error", err)
	// 			}
	// 		}()
	// 		return textOut, nil
	// 	},
	// 	genkit.WithFlowAuth(firebaseAuth),
	// )

	go func() {
		err := genkit.Init(ctx, &genkit.Options{FlowAddr: "-"})
		if err != nil {
			log.Fatalf("failed to initialize genkit: %v", err)
		}
	}()

	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{
			"http://localhost:3000",
			"http://localhost:4173",
			"https://assistant.heyditto.ai",
			"https://ditto-app-dev.web.app",
		},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"}, // Allow all headers
		MaxAge:         86400,         // 24 hours
	})
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/prompt", func(w http.ResponseWriter, r *http.Request) {
		ctx, err := firebaseAuth.ProvideAuthContext(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.PromptV1
		if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = firebaseAuth.CheckAuthPolicy(ctx, bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		user, err := db.GetOrCreateUser(ctx, bod.UserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if user.Balance <= 0 {
			http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
			return
		}

		var rsp claude.Response
		err = rsp.Prompt(ctx, bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for token := range rsp.Text {
			if token.Err != nil {
				http.Error(w, token.Err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, token.Ok)
		}

		receipt := db.Receipt{
			UserID:       user.ID,
			InputTokens:  int64(rsp.InputTokens),
			OutputTokens: int64(rsp.OutputTokens),
			ServiceName:  llm.ModelClaude35Sonnet,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})

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
		user, err := db.GetOrCreateUser(ctx, bod.UserID)
		if err != nil {
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

		receipt := db.Receipt{
			UserID:      user.ID,
			TotalTokens: int64(llm.EstimateTokens(bod.Text)),
			ServiceName: bod.Model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
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
		user, err := db.GetOrCreateUser(ctx, bod.UserID)
		if err != nil {
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
			receipt := db.Receipt{
				UserID:      user.ID,
				NumSearches: 1,
				ServiceName: llm.SearchEngineBrave,
			}
			if err := receipt.Insert(ctx); err != nil {
				slog.Error("failed to insert receipt", "error", err)
			}
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
		receipt := db.Receipt{
			UserID:      user.ID,
			NumSearches: 1,
			ServiceName: llm.SearchEngineGoogle,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})

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
			bod.Model = llm.ModelDalle3
		}
		user, err := db.GetOrCreateUser(ctx, bod.UserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if user.Balance <= 0 {
			http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
			return
		}

		type RequestImageOpenAI struct {
			Prompt string          `json:"prompt"`
			Model  llm.ServiceName `json:"model"`
			N      int             `json:"n"`
			Size   string          `json:"size"`
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
		resp, err := llm.HttpClient.Do(req)
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
		receipt := db.Receipt{
			UserID:      user.ID,
			NumImages:   1,
			ServiceName: bod.Model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
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
		err = firebaseAuth.CheckAuthPolicy(ctx, bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
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

	mux.HandleFunc("GET /v1/balance", func(w http.ResponseWriter, r *http.Request) {
		ctx, err := firebaseAuth.ProvideAuthContext(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var bod rq.BalanceV1
		if err := bod.FromQuery(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = firebaseAuth.CheckAuthPolicy(ctx, bod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var q struct {
			Balance   int64   `db:"balance"`
			ImgTokens float64 `db:"base_cost_per_call"`
		}
		err = db.D.QueryRowContext(ctx, `
			SELECT users.balance,
			       ROUND(users.balance / ((svc.base_cost_per_call + svc.base_cost_per_image) * tpd.count * (1 + svc.profit_margin_percentage / 100.0)))
			FROM users,
			     services AS svc,
			     tokens_per_dollar AS tpd
			WHERE uid = $1
			  AND svc.name = 'dall-e-3'
			  AND tpd.name = 'ditto'
		`, bod.UserID).Scan(&q.Balance, &q.ImgTokens)
		if err != nil {
			slog.Error("failed to get balance", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var rsp rp.BalanceV1
		rsp.Balance = numfmt.FormatLargeNumber(q.Balance)
		rsp.Images = numfmt.FormatLargeNumber(int64(q.ImgTokens))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rsp)
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
