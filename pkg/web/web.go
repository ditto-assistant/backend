package web

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/web/templates"
	datastar "github.com/starfederation/datastar/sdk/go"
	"github.com/yuin/goldmark"
)

type Client struct {
	cl *core.Client
	md goldmark.Markdown
}

func NewClient(cl *core.Client) *Client {
	return &Client{
		cl: cl,
		md: goldmark.New(),
	}
}

func (cl *Client) Routes(mux *http.ServeMux) {
	mux.Handle("/", templ.Handler(templates.Index()))

	mux.HandleFunc("/templates/v1/text-stream", func(w http.ResponseWriter, r *http.Request) {
		repeat, _ := strconv.ParseBool(r.URL.Query().Get("repeat"))
		// Create a new Datastar SSE instance
		sse := datastar.NewSSE(w, r)
		if !repeat {
			sse.MergeFragmentTempl(templates.TextStream())
		}

		// Initialize signals
		sse.MergeSignals([]byte(`{
			"streamStatus": "waiting",
			"currentLine": 0,
			"totalLines": 5
		}`))

		// Example markdown content
		markdownLines := []string{
			"# Streaming Markdown\nThis is line **1** with some *italic* text.",
			"## Code Example\n```python\nprint('Hello from line 2!')\nprint('Hello from line 2!')\n```",
			"> This is line 3 with a blockquote\n\nAnd some regular text.",
			"* Line 4 is a list item\n* With multiple points\n* And formatting **bold**",
			"### Final Line\nLine 5 with a [link](https://example.com) and `inline code`",
		}

		// Send messages
		for i, markdown := range markdownLines {
			select {
			case <-r.Context().Done():
				return
			default:
				var buf bytes.Buffer
				if err := cl.md.Convert([]byte(markdown), &buf); err != nil {
					sse.MergeFragments(fmt.Sprintf("<div class=\"error\">Error rendering markdown: %s</div>", err), datastar.WithSelectorID("stream-content"))
					continue
				}

				// Update signals
				sse.MergeSignals([]byte(fmt.Sprintf(`{
					"streamStatus": "streaming",
					"currentLine": %d
				}`, i+1)))

				// Merge the new content
				sse.MergeFragments(buf.String(), datastar.WithMergeAppend(), datastar.WithSelectorID("stream-content"))
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Send completion message
		var buf bytes.Buffer
		cl.md.Convert([]byte("## 🎉 Streaming Complete!\n*All markdown content has been delivered successfully.*"), &buf)

		// Update final status
		sse.MergeSignals([]byte(`{
			"streamStatus": "complete",
			"currentLine": 5
		}`))

		// Send final content
		sse.MergeFragments(buf.String(), datastar.WithMergeAppend(), datastar.WithSelectorID("stream-content"))
	})
}
