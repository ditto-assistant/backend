package google

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/search"
	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type Service struct {
	customSearch *customsearch.Service
}

var _ search.Service = (*Service)(nil)

func NewService(ctx context.Context) (svc *Service, err error) {
	customSearch, err := customsearch.NewService(ctx, option.WithAPIKey(secr.SEARCH_API_KEY.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize custom search: %w", err)
	}
	return &Service{customSearch: customSearch}, nil
}

func (s *Service) Search(ctx context.Context, req search.Request) (results search.Results, err error) {
	ser, err := s.customSearch.Cse.List().Do(
		googleapi.QueryParameter("q", req.Query),
		googleapi.QueryParameter("num", strconv.Itoa(req.NumResults)),
		googleapi.QueryParameter("cx", envs.SEARCH_ENGINE_ID),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
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
