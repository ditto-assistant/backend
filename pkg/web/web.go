package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/llama"
	"github.com/ditto-assistant/backend/pkg/web/templates"
	"github.com/ditto-assistant/backend/types/rq"
	datastar "github.com/starfederation/datastar/sdk/go"
)

type Client struct {
	cl *core.Client
}

func NewClient(cl *core.Client) *Client {
	return &Client{
		cl: cl,
	}
}

func (cl *Client) Routes(mux *http.ServeMux) {
	mux.Handle("/", templ.Handler(templates.Index()))

	mux.HandleFunc("/templates/v1/text-stream", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		repeat, _ := strconv.ParseBool(q.Get("repeat"))
		var dStarData struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(q.Get("datastar")), &dStarData); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Create a new Datastar SSE instance
		sse := datastar.NewSSE(w, r)
		if !repeat {
			sse.MergeFragmentTempl(templates.TextStream())
		}

		var rsp llm.StreamResponse
		llama.ModelLlama32.Prompt(r.Context(), rq.PromptV1{
			Model:      llm.ModelLlama32,
			UserPrompt: dStarData.Prompt,
		}, &rsp)

		i := 0
		for token := range rsp.Text {
			if token.Err != nil {
				sse.MergeSignals(fmt.Appendf(nil, `{
					"_streamStatus": "error",
					"_error": %q
				}`, token.Err))
				continue
			}

			select {
			case <-r.Context().Done():
				return
			default:

				sse.MergeSignals(fmt.Appendf(nil, `{
						"_streamStatus": "streaming",
						"_currentLine": %d,
						"_content": %q
					}`, i, token.Ok))

			}
		}

		// Add completion message to content
		finalMessage := "\n\n## 🎉 Streaming Complete!\n*All markdown content has been delivered successfully.*"

		// Send final status and content
		sse.MergeSignals(fmt.Appendf(nil, `{
			"_streamStatus": "complete",
			"_currentLine": %d,
			"_content": %q
		}`, i, finalMessage))
	})
}
