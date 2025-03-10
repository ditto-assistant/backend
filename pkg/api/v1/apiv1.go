package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/services/authfirebase"
	"github.com/ditto-assistant/backend/pkg/services/db"
	"github.com/ditto-assistant/backend/pkg/services/db/users"
	"github.com/ditto-assistant/backend/pkg/services/encryption"
	"github.com/ditto-assistant/backend/pkg/services/firestoremem"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai/dalle"
	"github.com/ditto-assistant/backend/pkg/services/search"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/ditto-assistant/backend/types/ty"
	"github.com/omniaura/mapcache"
)

func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/balance", s.Balance)
	mux.HandleFunc("GET /v1/conversations", s.GetConversations)
	mux.HandleFunc("POST /v1/google-search", s.WebSearch)
	mux.HandleFunc("POST /v1/generate-image", s.GenerateImage)
	mux.HandleFunc("POST /v1/presign-url", s.PresignURL)
	mux.HandleFunc("POST /v1/create-upload-url", s.CreateUploadURL)
	mux.HandleFunc("POST /v1/get-memories", s.GetMemories)
	mux.HandleFunc("POST /v1/feedback", s.Feedback)
	mux.HandleFunc("POST /v1/embed", s.Embed)
	mux.HandleFunc("POST /v1/search-examples", s.SearchExamples)
	mux.HandleFunc("POST /v1/create-prompt", s.CreatePrompt)
	mux.HandleFunc("POST /v1/save-response", s.SaveResponse)
}

const presignTTL = 24 * time.Hour

type Service struct {
	sd                ty.ShutdownContext
	sc                *core.Client
	searchClient      *search.Client
	urlCache          *mapcache.MapCache[string, string]
	dalle             *dalle.Client
	encryptionService *encryption.Service
}

type ServiceClients struct {
	SearchClient      *search.Client
	Dalle             *dalle.Client
	EncryptionService *encryption.Service
}

func NewService(sd ty.ShutdownContext, sc *core.Client, setup ServiceClients) *Service {
	urlCache, err := mapcache.New[string, string](
		mapcache.WithTTL(presignTTL/2),
		mapcache.WithCleanup(sd.Background, presignTTL),
	)
	if err != nil {
		panic(err)
	}
	return &Service{
		sd:                sd,
		sc:                sc,
		searchClient:      setup.SearchClient,
		urlCache:          urlCache,
		dalle:             setup.Dalle,
		encryptionService: setup.EncryptionService,
	}
}

// - MARK: balance

func (s *Service) Balance(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.BalanceV1
	if err := bod.FromQuery(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	rsp, err := users.GetBalance(r, db.D, bod)
	if err != nil {
		slog.Error("failed to handle balance request", "uid", bod.UserID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rsp)
}

// - MARK: web-search

func (s *Service) WebSearch(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
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
	err = tok.Check(bod.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if bod.NumResults == 0 {
		bod.NumResults = 5
	}
	user := users.User{UID: bod.UserID}
	ctx := r.Context()
	if err := user.GetByUID(ctx, db.D); err != nil {
		slog.Error("failed to get user", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user.Balance <= 0 {
		http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
		return
	}
	searchRequest := search.Request{
		User:       user,
		Query:      bod.Query,
		NumResults: bod.NumResults,
	}
	search, err := s.searchClient.Search(ctx, searchRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	search.Text(w)
}

// - MARK: generate-image

func (s *Service) GenerateImage(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.GenerateImageV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
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
	slog := slog.With("userID", bod.UserID, "model", bod.Model, "email", user.Email.String)
	if user.Balance <= 0 {
		http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
		return
	}
	if bod.DummyMode {
		fmt.Fprint(w, envs.DALLE_E_DUMMY_LINK)
		return
	}
	url, err := s.dalle.Prompt(ctx, &bod)
	if err != nil {
		slog.Error("failed to generate image", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, url)

	s.sd.Run(func(ctx context.Context) {
		slog.Debug("image receipt", "url", url)
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
		put, err := s.sc.FileStorage.S3.PutObject(&s3.PutObjectInput{
			Bucket: bucketDittoContent,
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
	})
}

// - MARK: presign-url

var bucketDittoContent = aws.String(envs.DITTO_CONTENT_BUCKET)

func (s *Service) PresignURL(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
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
	err = tok.Check(bod.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	url, err := s.urlCache.Get(bod.URL, func() (string, error) {
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
		objReq, _ := s.sc.FileStorage.S3.GetObjectRequest(&s3.GetObjectInput{
			Bucket: bucketDittoContent,
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
	fmt.Fprint(w, url)
}

// - MARK: create-upload-url

func (s *Service) CreateUploadURL(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.CreateUploadURLV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	key := fmt.Sprintf("%s/uploads/%d", bod.UserID, time.Now().UnixNano())
	req, _ := s.sc.FileStorage.S3.PutObjectRequest(&s3.PutObjectInput{
		Bucket: bucketDittoContent,
		Key:    aws.String(key),
	})
	url, err := req.Presign(15 * time.Minute)
	slog.Debug("created upload URL", "url", url)
	if err != nil {
		slog.Error("failed to generate upload URL", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, url)
}

// - MARK: get-memories

func (s *Service) GetMemories(w http.ResponseWriter, r *http.Request) {
	slog := slog.With("handler", "GetMemories")
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var req rq.GetMemoriesV1
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("Failed to decode request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	err = tok.Check(req.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	slog = slog.With("userID", req.UserID)
	if len(req.Vector) == 0 {
		slog.Error("Missing required parameters",
			"userId", req.UserID != "",
			"vector", req.Vector != nil)
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}
	if len(req.Vector) > 2048 {
		slog.Error("Vector dimension exceeds maximum allowed (2048)")
		http.Error(w, "Vector dimension exceeds maximum allowed (2048)", http.StatusBadRequest)
		return
	}
	if req.K == 0 {
		req.K = 5
	}
	if req.K > 100 {
		slog.Error("Number of requested neighbors exceeds maximum allowed (100)")
		http.Error(w, "Number of requested neighbors exceeds maximum allowed (100)", http.StatusBadRequest)
		return
	}
	memories, err := s.sc.Memories.GetMemoriesV2(r.Context(), &rq.GetMemoriesV2{
		UserID:      req.UserID,
		StripImages: false,
		LongTerm: &rq.ParamsLongTermMemoriesV2{
			Vector:     req.Vector,
			NodeCounts: []int{req.K},
		},
	})
	if err != nil {
		slog.Error("Failed to get memories", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rp.MemoriesV1{Memories: memories.LongTerm})
}

// - MARK: feedback

func (s *Service) Feedback(w http.ResponseWriter, r *http.Request) {
	slog := slog.With("path", "v1/feedback")
	tok, err := s.sc.Auth.VerifyTokenFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.FeedbackV1
	if err := bod.FromForm(r); err != nil {
		slog.Debug("failed to parse request", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	u := users.User{UID: bod.UserID}
	if err := u.GetByUID(r.Context(), db.D); err != nil {
		slog.Error("failed to get user", "error", err, "userID", bod.UserID)
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	device := users.UserDevice{
		UserID:    u.ID,
		DeviceUID: bod.DeviceID,
		Version:   bod.Version,
	}
	exists, err := device.Exists(r.Context(), db.D)
	if err != nil {
		slog.Error("failed to get device", "error", err, "deviceID", bod.DeviceID)
		http.Error(w, "Invalid device ID", http.StatusBadRequest)
		return
	}
	if !exists {
		slog.Error("device not found", "deviceID", bod.DeviceID)
		http.Error(w, "Invalid device ID", http.StatusBadRequest)
		return
	}
	feedback := users.UserFeedback{
		DeviceID: device.ID,
		Type:     bod.Type,
		Feedback: bod.Feedback,
	}
	if err := feedback.Insert(r.Context(), db.D); err != nil {
		slog.Error("failed to insert feedback", "error", err, "request", bod)
		http.Error(w, "Failed to insert feedback", http.StatusInternalServerError)
		return
	}
	slog.Debug("feedback inserted",
		"email", u.Email.String,
		"version", bod.Version,
		"type", bod.Type,
		"feedback", bod.Feedback)
	w.WriteHeader(http.StatusCreated)
}

// - MARK: embed

func (s *Service) Embed(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.EmbedV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
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
	var tokens int64
	if bod.Model == llm.ModelTextEmbedding3Small {
		embedding, err = openai.GenerateEmbedding(ctx, bod.Text, bod.Model)
		tokens = int64(llm.EstimateTokens(bod.Text))
	} else {
		embedding, tokens, err = s.sc.Embedder.EmbedSingle(ctx, bod.Text, bod.Model)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(embedding)
	s.sd.Run(func(ctx context.Context) {
		slog.Debug("receipt", "input_tokens", tokens)
		receipt := db.Receipt{
			UserID:      user.ID,
			TotalTokens: tokens,
			ServiceName: bod.Model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})
}

// - MARK: search-examples

func (s *Service) SearchExamples(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.SearchExamplesV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
	}
	if bod.K == 0 {
		bod.K = 5
	}
	ctx := r.Context()
	if len(bod.Embedding) == 0 && bod.PairID == "" {
		http.Error(w, "embedding or pairID is required", http.StatusBadRequest)
	}
	if bod.PairID != "" {
		embedding, err := s.sc.Memories.GetEmbeddingPrompt(ctx, bod.UserID, bod.PairID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		bod.Embedding = llm.Embedding(embedding)
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
}

// - MARK: create-prompt

func (s *Service) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Check for encrypted prompt request
	if r.Header.Get("Content-Type") == "application/json+encrypted" {
		s.createEncryptedPrompt(w, r, tok)
		return
	}

	// Standard unencrypted prompt
	var bod rq.CreatePromptV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
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
	slog := slog.With("action", "embed", "userID", bod.UserID, "email", user.Email.String)
	model := llm.ModelTextEmbedding005
	embedding, tokens, err := s.sc.Embedder.EmbedSingle(ctx, bod.Prompt, model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, err := s.sc.Memories.CreatePrompt(ctx, bod.UserID, &firestoremem.CreatePromptRequest{
		DeviceID:         bod.DeviceID,
		Prompt:           bod.Prompt,
		EmbeddingPrompt5: firestore.Vector32(embedding),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(id))
	s.sd.Run(func(ctx context.Context) {
		slog.Debug("receipt", "input_tokens", tokens)
		receipt := db.Receipt{
			UserID:      user.ID,
			TotalTokens: tokens,
			ServiceName: model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})
}

// createEncryptedPrompt handles the creation of an encrypted prompt
func (s *Service) createEncryptedPrompt(w http.ResponseWriter, r *http.Request, tok *authfirebase.AuthToken) {
	var encReq rq.CreateEncryptedPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&encReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := tok.Check(encReq.UserID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	// Validate encryption request
	if encReq.EncryptedPrompt == "" || encReq.UnencryptedPrompt == "" || encReq.EncryptionKeyID == "" {
		http.Error(w, "EncryptedPrompt, UnencryptedPrompt, and EncryptionKeyID are required", http.StatusBadRequest)
		return
	}

	user := users.User{UID: encReq.UserID}
	ctx := r.Context()
	if err := user.GetByUID(ctx, db.D); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Verify encryption key
	if s.encryptionService != nil {
		_, err := s.encryptionService.GetKey(ctx, user.ID, encReq.EncryptionKeyID)
		if err != nil {
			slog.Error("Error verifying encryption key", "error", err, "user_id", encReq.UserID)
			http.Error(w, "Invalid encryption key", http.StatusBadRequest)
			return
		}
	}

	slog := slog.With("action", "embed-encrypted", "userID", encReq.UserID, "email", user.Email.String)

	// Generate embedding from unencrypted text (for search)
	model := llm.ModelTextEmbedding005
	embedding, tokens, err := s.sc.Embedder.EmbedSingle(ctx, encReq.UnencryptedPrompt, model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create prompt with both encrypted and unencrypted versions
	deviceID := r.Header.Get("X-Device-ID")
	id, err := s.sc.Memories.CreatePrompt(ctx, encReq.UserID, &firestoremem.CreatePromptRequest{
		DeviceID:          deviceID,
		Prompt:            encReq.UnencryptedPrompt, // For embeddings
		EncryptedPrompt:   encReq.EncryptedPrompt,   // For storage
		EncryptionKeyID:   encReq.EncryptionKeyID,
		EncryptionVersion: encReq.EncryptionVersion,
		IsEncrypted:       true,
		EmbeddingPrompt5:  firestore.Vector32(embedding),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return prompt ID
	resp := rp.CreateEncryptedPromptResponse{
		PromptID: id,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	// Record token usage
	s.sd.Run(func(ctx context.Context) {
		slog.Debug("receipt", "input_tokens", tokens)
		receipt := db.Receipt{
			UserID:      user.ID,
			TotalTokens: tokens,
			ServiceName: model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})
}

// - MARK: save-response

func (s *Service) SaveResponse(w http.ResponseWriter, r *http.Request) {
	slog := slog.With("path", "v1/save-response")
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Check for encrypted response request
	if r.Header.Get("Content-Type") == "application/json+encrypted" {
		s.saveEncryptedResponse(w, r, slog, tok)
		return
	}

	// Standard unencrypted response
	var bod rq.SaveResponseV1
	if err := json.NewDecoder(r.Body).Decode(&bod); err != nil {
		slog.Error("Failed to decode request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	err = tok.Check(bod.UserID)
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
	slog = slog.With("userID", bod.UserID)
	model := llm.ModelTextEmbedding005
	embedding, tokens, err := s.sc.Embedder.EmbedSingle(ctx, bod.Response, model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = s.sc.Memories.SaveResponse(r.Context(), &firestoremem.SaveResponseRequest{
		UserID:             bod.UserID,
		PairID:             bod.PairID,
		Response:           bod.Response,
		EmbeddingResponse5: firestore.Vector32(embedding),
	})
	if err != nil {
		slog.Error("Failed to save response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	s.sd.Run(func(ctx context.Context) {
		slog.Debug("receipt", "input_tokens", tokens)
		receipt := db.Receipt{
			UserID:      user.ID,
			TotalTokens: tokens,
			ServiceName: model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})
}

// saveEncryptedResponse handles saving an encrypted response
func (s *Service) saveEncryptedResponse(w http.ResponseWriter, r *http.Request, slog *slog.Logger, tok *authfirebase.AuthToken) {
	var encReq rq.SaveEncryptedResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&encReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userUID := encReq.UserID
	if err := tok.Check(userUID); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate encryption request
	if encReq.EncryptedResponse == "" || encReq.UnencryptedResponse == "" || encReq.PromptID == "" || encReq.EncryptionKeyID == "" {
		http.Error(w, "EncryptedResponse, UnencryptedResponse, PromptID, and EncryptionKeyID are required", http.StatusBadRequest)
		return
	}

	user := users.User{UID: userUID}
	ctx := r.Context()
	if err := user.GetByUID(ctx, db.D); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Verify encryption key
	if s.encryptionService != nil {
		_, err := s.encryptionService.GetKey(ctx, user.ID, encReq.EncryptionKeyID)
		if err != nil {
			slog.Error("Error verifying encryption key", "error", err, "user_id", userUID)
			http.Error(w, "Invalid encryption key", http.StatusBadRequest)
			return
		}
	}

	slog = slog.With("userID", userUID, "action", "save-encrypted-response")

	// Generate embedding from unencrypted text (for search)
	model := llm.ModelTextEmbedding005
	embedding, tokens, err := s.sc.Embedder.EmbedSingle(ctx, encReq.UnencryptedResponse, model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save both encrypted and unencrypted versions
	err = s.sc.Memories.SaveResponse(ctx, &firestoremem.SaveResponseRequest{
		UserID:             userUID,
		PairID:             encReq.PromptID,
		Response:           encReq.UnencryptedResponse, // For embeddings
		EncryptedResponse:  encReq.EncryptedResponse,   // For storage
		EncryptionKeyID:    encReq.EncryptionKeyID,
		EncryptionVersion:  encReq.EncryptionVersion,
		IsEncrypted:        true,
		EmbeddingResponse5: firestore.Vector32(embedding),
	})

	if err != nil {
		slog.Error("Failed to save encrypted response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success
	resp := rp.SaveEncryptedResponseResponse{
		Success: true,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)

	// Record token usage
	s.sd.Run(func(ctx context.Context) {
		slog.Debug("receipt", "input_tokens", tokens)
		receipt := db.Receipt{
			UserID:      user.ID,
			TotalTokens: tokens,
			ServiceName: model,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt", "error", err)
		}
	})
}

// - MARK: conversations

func (s *Service) GetConversations(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	userID := q.Get("userId")
	if userID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}
	err = tok.Check(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	limit := 20 // Default limit
	if limitStr := q.Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	var cursor string
	if cursorStr := q.Get("cursor"); cursorStr != "" {
		cursor = cursorStr
	}
	memoriesRef := s.sc.Memories.ConversationsRef(userID)
	query := memoriesRef.OrderBy("timestamp", firestore.Desc).Limit(limit + 1) // Get one extra to determine if there are more pages

	if cursor != "" {
		cursorDoc, err := memoriesRef.Doc(cursor).Get(r.Context())
		if err != nil {
			http.Error(w, "Invalid cursor", http.StatusBadRequest)
			return
		}
		query = query.StartAfter(cursorDoc)
	}
	docs, err := query.Documents(r.Context()).GetAll()
	if err != nil {
		slog.Error("failed to query conversations", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hasNextPage := len(docs) > limit
	if hasNextPage {
		docs = docs[:limit] // Remove the extra document we fetched
	}

	messages := make([]rp.Memory, 0, len(docs))
	for _, doc := range docs {
		var mem rp.Memory
		if err := doc.DataTo(&mem); err != nil {
			slog.Error("failed to unmarshal memory", "error", err)
			continue
		}
		mem.ID = doc.Ref.ID
		mem.FormatResponse()
		if err := mem.PresignImages(r.Context(), userID, s.sc.FileStorage); err != nil {
			slog.Error("failed to presign images", "error", err)
			continue
		}
		messages = append(messages, mem)
	}

	nextCursor := ""
	if hasNextPage && len(messages) > 0 {
		nextCursor = messages[len(messages)-1].ID
	}

	response := struct {
		Messages   []rp.Memory `json:"messages"`
		NextCursor string      `json:"nextCursor,omitempty"`
	}{
		Messages:   messages,
		NextCursor: nextCursor,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
