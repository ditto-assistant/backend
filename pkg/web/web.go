package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"unicode"

	"github.com/a-h/templ"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/llama"
	"github.com/ditto-assistant/backend/pkg/web/templates"
	"github.com/ditto-assistant/backend/types/rq"
	datastar "github.com/starfederation/datastar/sdk/go"
)

// chunkSize is the target size for each chunk of text
const chunkSize = 50

// breakIntoChunks splits text into chunks, trying to break at word boundaries
func breakIntoChunks(text string, targetSize int) []string {
	if len(text) <= targetSize {
		return []string{text}
	}

	var chunks []string
	start := 0
	for start < len(text) {
		end := start + targetSize
		if end > len(text) {
			chunks = append(chunks, text[start:])
			break
		}

		// Try to find a word boundary
		for end > start && end < len(text) && !unicode.IsSpace(rune(text[end])) {
			end--
		}
		if end == start {
			// No word boundary found, just split at targetSize
			end = start + targetSize
		}

		chunks = append(chunks, text[start:end])
		start = end
		// Include all characters, even whitespace
	}
	return chunks
}

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
		repeat, _ := strconv.ParseBool(r.URL.Query().Get("repeat"))
		var dStarData struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("datastar")), &dStarData); err != nil {
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
				sse.MergeSignals([]byte(fmt.Sprintf(`{
					"_streamStatus": "error",
					"_error": %q
				}`, token.Err)))
				continue
			}

			select {
			case <-r.Context().Done():
				return
			default:
				// Break token into smaller chunks
				// chunks := breakIntoChunks(token.Ok, chunkSize)
				// for _, chunk := range chunks {
				// i++
				// Update signals with new content and status
				sse.MergeSignals([]byte(fmt.Sprintf(`{
						"_streamStatus": "streaming",
						"_currentLine": %d,
						"_content": %q
					}`, i, token.Ok)))
				// time.Sleep(50 * time.Millisecond) // Reduced delay for smoother updates
				// }
			}
		}

		// Add completion message to content
		finalMessage := "\n\n## 🎉 Streaming Complete!\n*All markdown content has been delivered successfully.*"

		// Send final status and content
		sse.MergeSignals([]byte(fmt.Sprintf(`{
			"_streamStatus": "complete",
			"_currentLine": %d,
			"_content": %q
		}`, i, finalMessage)))
	})
}
