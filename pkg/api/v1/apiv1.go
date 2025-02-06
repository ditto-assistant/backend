package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/services/db"
	"github.com/ditto-assistant/backend/pkg/services/db/users"
	"github.com/ditto-assistant/backend/pkg/services/llm/openai/dalle"
	"github.com/ditto-assistant/backend/pkg/services/search"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/ditto-assistant/backend/types/ty"
	"github.com/omniaura/mapcache"
)

const presignTTL = 24 * time.Hour

type Service struct {
	sd           ty.ShutdownContext
	sc           *core.Client
	searchClient *search.Client
	s3           *s3.S3
	urlCache     *mapcache.MapCache[string, string]
	dalle        *dalle.Client
}

type ServiceClients struct {
	SearchClient *search.Client
	S3           *s3.S3
	Dalle        *dalle.Client
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
		sd:           sd,
		sc:           sc,
		searchClient: setup.SearchClient,
		s3:           setup.S3,
		urlCache:     urlCache,
		dalle:        setup.Dalle,
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
	err = tok.Check(bod)
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
	slog := slog.With("userID", bod.UserID, "model", bod.Model, "email", user.Email.String)
	if user.Balance <= 0 {
		http.Error(w, fmt.Sprintf("user balance is: %d", user.Balance), http.StatusPaymentRequired)
		return
	}
	if bod.DummyMode {
		fmt.Fprintln(w, envs.DALLE_E_DUMMY_LINK)
		return
	}
	url, err := s.dalle.Prompt(ctx, &bod)
	if err != nil {
		slog.Error("failed to generate image", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, url)

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
		put, err := s.s3.PutObject(&s3.PutObjectInput{
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
	err = tok.Check(bod)
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
		objReq, _ := s.s3.GetObjectRequest(&s3.GetObjectInput{
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
	fmt.Fprintln(w, url)
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
	err = tok.Check(bod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	key := fmt.Sprintf("%s/uploads/%d", bod.UserID, time.Now().UnixNano())
	req, _ := s.s3.PutObjectRequest(&s3.PutObjectInput{
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
	fmt.Fprintln(w, url)
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
	err = tok.Check(req)
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
		http.Error(w, "Failed to get memories", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rp.MemoriesV1{Memories: memories.LongTerm})
}

// - MARK: feedback

func (s *Service) Feedback(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.Auth.VerifyToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var bod rq.FeedbackV1
	if err := bod.FromForm(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = tok.Check(bod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var u users.User
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
		slog.Error("failed to insert feedback",
			"error", err,
			"request", bod)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
