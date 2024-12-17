package brave

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/pkg/search"
	"github.com/ditto-assistant/backend/types/ty"
)

type Service struct {
	sc ty.ServiceContext
}

var _ search.Service = (*Service)(nil)

func NewService(sc ty.ServiceContext) (svc search.Service, err error) {
	return &Service{sc: sc}, nil
}

const basedURL = "https://api.search.brave.com/res/v1/web/search"

func (s *Service) Search(ctx context.Context, req search.Request) (search.Results, error) {
	q := url.Values{
		"q":     {req.Query},
		"count": {strconv.Itoa(req.NumResults)},
	}
	rr, err := http.NewRequestWithContext(ctx, "GET", basedURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	rr.Header.Add("X-Subscription-Token", secr.BRAVE_SEARCH_API_KEY.String())
	resp, err := http.DefaultClient.Do(rr)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave search returned status code %d", resp.StatusCode)
	}
	var results Results
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	s.sc.ShutdownWG.Add(1)
	go func() {
		defer s.sc.ShutdownWG.Done()
		ctx, cancel := context.WithTimeout(s.sc.Background, 15*time.Second)
		defer cancel()
		receipt := db.Receipt{
			UserID:      req.User.ID,
			NumSearches: 1,
			ServiceName: llm.SearchEngineBrave,
		}
		if err := receipt.Insert(ctx); err != nil {
			slog.Error("failed to insert receipt for brave search", "error", err)
		}
		slog.Debug("brave search completed",
			"user_id", req.User.ID,
			"balance", req.User.Balance,
			"total_tokens_airdropped", req.User.TotalTokensAirdropped,
			"service", llm.SearchEngineBrave,
			"receipt_id", receipt.ID,
			"service_id", receipt.ServiceID,
			"num_searches", receipt.NumSearches,
		)
	}()
	return results, nil
}

func (r Results) Text(w io.Writer) error {
	if len(r.Web.Results) == 0 && len(r.Videos.Results) == 0 {
		fmt.Fprintln(w, "No results found")
		return nil
	}
	// Video Results
	fmt.Fprintln(w, "Video Results:")
	if len(r.Videos.Results) == 0 {
		fmt.Fprintln(w, "  No video results found")
	} else {
		for i, result := range r.Videos.Results {
			fmt.Fprintf(w, "%d. [%s](%s)\n", i+1, result.Title, result.URL)
			fmt.Fprintf(w, "   Description: %s\n", result.Description)
			if result.Age != "" {
				fmt.Fprintf(w, "   Age: %s\n", result.Age)
			}
			fmt.Fprintf(w, "   Source: %s\n", result.MetaURL.Hostname)
			fmt.Fprintf(w, "   Thumbnail: %s\n\n", result.Thumbnail.Src)
		}
	}
	// Web Results
	fmt.Fprintln(w, "\nWeb Results:")
	if len(r.Web.Results) == 0 {
		fmt.Fprintln(w, "  No web results found")
	} else {
		for i, result := range r.Web.Results {
			fmt.Fprintf(w, "%d. [%s](%s)\n", i+1, result.Title, result.URL)
			fmt.Fprintf(w, "   Description: %s\n", result.Description)
			if result.Age != "" {
				fmt.Fprintf(w, "   Age: %s\n", result.Age)
			}
			fmt.Fprintf(w, "   Language: %s\n", result.Language)
			fmt.Fprintf(w, "   Source: %s\n", result.MetaURL.Hostname)
			if result.Profile.Name != "" {
				fmt.Fprintf(w, "   Profile: %s (%s)\n", result.Profile.Name, result.Profile.URL)
			}
			fmt.Fprintf(w, "   Family Friendly: %v\n", result.FamilyFriendly)
			if result.Thumbnail.Src != "" {
				fmt.Fprintf(w, "   Thumbnail: %s\n", result.Thumbnail.Src)
			}
			if len(result.ExtraSnippets) > 0 {
				fmt.Fprintln(w, "   Additional Information:")
				for _, snippet := range result.ExtraSnippets {
					fmt.Fprintf(w, "    - %s\n", snippet)
				}
			}
			fmt.Fprintln(w)
		}
	}

	return nil
}

type Results struct {
	Query  Query       `json:"query"`
	Mixed  Mixed       `json:"mixed"`
	Type   string      `json:"type"`
	Videos VideosClass `json:"videos"`
	Web    WebClass    `json:"web"`
}

type Mixed struct {
	Type string `json:"type"`
	Main []Main `json:"main"`
	// Top  []interface{} `json:"top"`
	// Side []interface{} `json:"side"`
}

type Main struct {
	Type  Type   `json:"type"`
	Index *int64 `json:"index,omitempty"`
	All   bool   `json:"all"`
}

type Query struct {
	Original             string `json:"original"`
	ShowStrictWarning    bool   `json:"show_strict_warning"`
	IsNavigational       bool   `json:"is_navigational"`
	IsNewsBreaking       bool   `json:"is_news_breaking"`
	SpellcheckOff        bool   `json:"spellcheck_off"`
	Country              string `json:"country"`
	BadResults           bool   `json:"bad_results"`
	ShouldFallback       bool   `json:"should_fallback"`
	PostalCode           string `json:"postal_code"`
	City                 string `json:"city"`
	HeaderCountry        string `json:"header_country"`
	MoreResultsAvailable bool   `json:"more_results_available"`
	State                string `json:"state"`
}

type VideosClass struct {
	Type             Type           `json:"type"`
	Results          []VideosResult `json:"results"`
	MutatedByGoggles bool           `json:"mutated_by_goggles"`
}

type VideosResult struct {
	Type        string          `json:"type"`
	URL         string          `json:"url"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Age         string          `json:"age,omitempty"`
	Video       Video           `json:"video"`
	MetaURL     MetaURL         `json:"meta_url"`
	Thumbnail   PurpleThumbnail `json:"thumbnail"`
}

type MetaURL struct {
	Scheme   string `json:"scheme"`
	Netloc   string `json:"netloc"`
	Hostname string `json:"hostname"`
	Favicon  string `json:"favicon"`
	Path     string `json:"path"`
}

type PurpleThumbnail struct {
	Src      string `json:"src"`
	Original string `json:"original"`
}

type Video struct {
}

type WebClass struct {
	Type           string      `json:"type"`
	Results        []WebResult `json:"results"`
	FamilyFriendly bool        `json:"family_friendly"`
}

type WebResult struct {
	Title          string          `json:"title"`
	URL            string          `json:"url"`
	IsSourceLocal  bool            `json:"is_source_local"`
	IsSourceBoth   bool            `json:"is_source_both"`
	Description    string          `json:"description"`
	Profile        Profile         `json:"profile"`
	Language       string          `json:"language"`
	FamilyFriendly bool            `json:"family_friendly"`
	Type           string          `json:"type"`
	Subtype        string          `json:"subtype"`
	MetaURL        MetaURL         `json:"meta_url"`
	Thumbnail      FluffyThumbnail `json:"thumbnail"`
	Age            string          `json:"age"`
	ExtraSnippets  []string        `json:"extra_snippets"`
}

type Profile struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	LongName string `json:"long_name"`
	Img      string `json:"img"`
}

type FluffyThumbnail struct {
	Src      string `json:"src"`
	Original string `json:"original"`
	Logo     bool   `json:"logo"`
}

type Type string

const (
	Videos Type = "videos"
	Web    Type = "web"
)
