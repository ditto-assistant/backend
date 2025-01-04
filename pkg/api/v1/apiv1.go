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

	"cloud.google.com/go/firestore"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/db/users"
	"github.com/ditto-assistant/backend/pkg/llm/openai/dalle"
	"github.com/ditto-assistant/backend/pkg/search"
	"github.com/ditto-assistant/backend/pkg/service"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"github.com/omniaura/mapcache"
)

const presignTTL = 24 * time.Hour

type Service struct {
	sc           service.Context
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

func NewService(sc service.Context, setup ServiceClients) *Service {
	urlCache, err := mapcache.New[string, string](
		mapcache.WithTTL(presignTTL/2),
		mapcache.WithCleanup(sc.Background, presignTTL),
	)
	if err != nil {
		panic(err)
	}
	return &Service{
		sc:           sc,
		searchClient: setup.SearchClient,
		s3:           setup.S3,
		urlCache:     urlCache,
		dalle:        setup.Dalle,
	}
}

// - MARK: balance

func (s *Service) Balance(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.App.VerifyToken(r)
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
	rsp, err := users.GetBalance(r.Context(), bod)
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
	tok, err := s.sc.App.VerifyToken(r)
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
	tok, err := s.sc.App.VerifyToken(r)
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
	url, err := s.dalle.Prompt(ctx, &bod)
	if err != nil {
		slog.Error("failed to generate image", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, url)
	slog.Debug("generated image", "url", url)

	s.sc.ShutdownWG.Add(1)
	go func() {
		defer s.sc.ShutdownWG.Done()
		ctx, cancel := context.WithTimeout(s.sc.Background, 15*time.Second)
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
	}()
}

// - MARK: presign-url

var bucketDittoContent = aws.String(envs.DITTO_CONTENT_BUCKET)

func (s *Service) PresignURL(w http.ResponseWriter, r *http.Request) {
	tok, err := s.sc.App.VerifyToken(r)
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
	tok, err := s.sc.App.VerifyToken(r)
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
	tok, err := s.sc.App.VerifyToken(r)
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
	slog = slog.With("user_id", req.UserID)
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
	fs := s.sc.App.Firestore
	memoriesRef := fs.Collection("memory").Doc(req.UserID).Collection("conversations")
	vectorQuery := memoriesRef.FindNearest("embedding_vector",
		req.Vector,
		req.K,
		firestore.DistanceMeasureCosine,
		&firestore.FindNearestOptions{
			DistanceResultField: "vector_distance",
		})
	querySnapshot, err := vectorQuery.Documents(r.Context()).GetAll()
	if err != nil {
		slog.Error("Failed to execute vector query", "error", err)
		http.Error(w, "Failed to search memories", http.StatusInternalServerError)
		return
	}

	memories := make([]rp.Memory, 0, len(querySnapshot))
	for _, doc := range querySnapshot {
		var data struct {
			Prompt         string    `firestore:"prompt"`
			Response       string    `firestore:"response"`
			Timestamp      time.Time `firestore:"timestamp"`
			VectorDistance float32   `firestore:"vector_distance"`
		}
		if err := doc.DataTo(&data); err != nil {
			slog.Error("Failed to unmarshal document", "error", err, "docID", doc.Ref.ID)
			continue
		}
		similarityScore := 1 - data.VectorDistance

		prompt := s.processImageLinks(r.Context(), req.UserID, data.Prompt, slog)
		response := s.processImageLinks(r.Context(), req.UserID, data.Response, slog)

		memories = append(memories, rp.Memory{
			ID:             doc.Ref.ID,
			Score:          similarityScore,
			Prompt:         prompt,
			Response:       response,
			Timestamp:      data.Timestamp,
			VectorDistance: data.VectorDistance,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rp.MemoriesV1{Memories: memories})
}

// processImageLinks replaces image URLs with presigned URLs for backblaze and dalle images
func (s *Service) processImageLinks(_ context.Context, userID, text string, slog *slog.Logger) string {
	const (
		prefixImageAttachment      = "![image]("
		prefixDittoImageAttachment = "![DittoImage]("
		suffixImageAttachment      = ")"
	)

	result := text
	searchText := text

	// Process ![image]() links
	for {
		imgIdx := strings.Index(searchText, prefixImageAttachment)
		if imgIdx == -1 {
			break
		}
		start := imgIdx + len(prefixImageAttachment)
		if start >= len(searchText) {
			break
		}
		afterPrefix := searchText[start:]
		closeIdx := strings.Index(afterPrefix, suffixImageAttachment)
		if closeIdx == -1 {
			break
		}
		url := afterPrefix[:closeIdx]

		// Calculate absolute positions in the result string
		resultImgIdx := strings.Index(result, searchText[imgIdx:start+closeIdx+1])
		if resultImgIdx == -1 {
			searchText = searchText[start+closeIdx+1:]
			continue
		}

		if strings.HasPrefix(url, envs.DITTO_CONTENT_PREFIX) || strings.HasPrefix(url, envs.DALL_E_PREFIX) {
			presignedURL, err := s.urlCache.Get(url, func() (string, error) {
				urlParts := strings.Split(url, "?")
				if len(urlParts) == 0 {
					return "", fmt.Errorf("failed to get filename from URL: %s", url)
				}
				filename := strings.TrimPrefix(urlParts[0], envs.DITTO_CONTENT_PREFIX)
				filename = strings.TrimPrefix(filename, envs.DALL_E_PREFIX)
				filename = strings.TrimPrefix(filename, userID+"/")
				filename = strings.TrimPrefix(filename, "generated-images/") // Remove any existing folder prefix
				key := fmt.Sprintf("%s/generated-images/%s", userID, filename)
				objReq, _ := s.s3.GetObjectRequest(&s3.GetObjectInput{
					Bucket: bucketDittoContent,
					Key:    aws.String(key),
				})
				return objReq.Presign(presignTTL)
			})
			if err == nil {
				slog.Debug("Replaced image URL", "original", url)
				result = result[:resultImgIdx] + prefixImageAttachment + presignedURL + suffixImageAttachment + result[resultImgIdx+len(prefixImageAttachment)+len(url)+len(suffixImageAttachment):]
			}
		}
		// Always advance the search text
		searchText = searchText[start+closeIdx+1:]
	}

	// Process ![DittoImage]() links
	searchText = result
	for {
		imgIdx := strings.Index(searchText, prefixDittoImageAttachment)
		if imgIdx == -1 {
			break
		}
		start := imgIdx + len(prefixDittoImageAttachment)
		if start >= len(searchText) {
			break
		}
		afterPrefix := searchText[start:]
		closeIdx := strings.Index(afterPrefix, suffixImageAttachment)
		if closeIdx == -1 {
			break
		}
		url := afterPrefix[:closeIdx]

		// Calculate absolute positions in the result string
		resultImgIdx := strings.Index(result, searchText[imgIdx:start+closeIdx+1])
		if resultImgIdx == -1 {
			searchText = searchText[start+closeIdx+1:]
			continue
		}

		if strings.HasPrefix(url, envs.DITTO_CONTENT_PREFIX) || strings.HasPrefix(url, envs.DALL_E_PREFIX) {
			presignedURL, err := s.urlCache.Get(url, func() (string, error) {
				urlParts := strings.Split(url, "?")
				if len(urlParts) == 0 {
					return "", fmt.Errorf("failed to get filename from URL: %s", url)
				}
				filename := strings.TrimPrefix(urlParts[0], envs.DITTO_CONTENT_PREFIX)
				filename = strings.TrimPrefix(filename, envs.DALL_E_PREFIX)
				filename = strings.TrimPrefix(filename, userID+"/")
				filename = strings.TrimPrefix(filename, "generated-images/") // Remove any existing folder prefix
				key := fmt.Sprintf("%s/generated-images/%s", userID, filename)
				objReq, _ := s.s3.GetObjectRequest(&s3.GetObjectInput{
					Bucket: bucketDittoContent,
					Key:    aws.String(key),
				})
				return objReq.Presign(presignTTL)
			})
			if err == nil {
				slog.Debug("Replaced DittoImage URL", "new", presignedURL)
				result = result[:resultImgIdx] + prefixDittoImageAttachment + presignedURL + suffixImageAttachment + result[resultImgIdx+len(prefixDittoImageAttachment)+len(url)+len(suffixImageAttachment):]
			}
		}
		// Always advance the search text
		searchText = searchText[start+closeIdx+1:]
	}

	return result
}
