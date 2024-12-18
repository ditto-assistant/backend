package google

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/pkg/search"
	"github.com/ditto-assistant/backend/pkg/service"
	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type Service struct {
	mu           sync.RWMutex
	customSearch *customsearch.Service
	sc           service.Context
}

var _ search.Service = (*Service)(nil)

const ApiKeyID = "SEARCH_API_KEY"

func NewService(sc service.Context) *Service {
	return &Service{sc: sc}
}

func (s *Service) setup(ctx context.Context) error {
	s.mu.RLock()
	svc := s.customSearch
	s.mu.RUnlock()
	if svc != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.customSearch != nil {
		return nil
	}
	key, err := s.sc.Secr.Fetch(ctx, ApiKeyID)
	if err != nil {
		return fmt.Errorf("failed to fetch api key: %w", err)
	}
	s.customSearch, err = customsearch.NewService(s.sc.Background, option.WithAPIKey(key))
	if err != nil {
		return fmt.Errorf("failed to initialize custom search: %w", err)
	}
	slog.Debug("google search initialized", "secret_id", ApiKeyID)
	return nil
}

func (s *Service) Search(ctx context.Context, req search.Request) (results search.Results, err error) {
	if err := s.setup(ctx); err != nil {
		return nil, fmt.Errorf("failed to setup custom search: %w", err)
	}
	ser, err := s.customSearch.Cse.List().Do(
		googleapi.QueryParameter("q", req.Query),
		googleapi.QueryParameter("num", strconv.Itoa(req.NumResults)),
		googleapi.QueryParameter("cx", envs.SEARCH_ENGINE_ID),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	s.sc.ShutdownWG.Add(1)
	go func() {
		defer s.sc.ShutdownWG.Done()
		ctx, cancel := context.WithTimeout(s.sc.Background, 15*time.Second)
		defer cancel()
		receipt := db.Receipt{
			UserID:      req.User.ID,
			NumSearches: 1,
			ServiceName: llm.SearchEngineGoogle,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt for google search", "error", err)
		}
		slog.Debug("google search completed",
			"user_id", req.User.ID,
			"balance", req.User.Balance,
			"service", llm.SearchEngineGoogle,
			"receipt_id", receipt.ID,
			"service_id", receipt.ServiceID,
			"num_searches", receipt.NumSearches,
		)
	}()
	return &Results{Items: ser.Items}, nil
}

type Results struct {
	Items []*customsearch.Result
}

func (r *Results) Text(w io.Writer) error {
	if len(r.Items) == 0 {
		w.Write([]byte("No results found"))
		return nil
	}
	for i, item := range r.Items {
		fmt.Fprintf(w,
			"%d. [%s](%s)\n\t- %s\n\n", i+1,
			item.Title, item.Link, item.Snippet,
		)
	}
	return nil
}
