package web

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/web/templates"
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

func writeSSELines(w http.ResponseWriter, content string) {
	// Split content into lines and write each with data: prefix
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprintf(w, "\n")
}

func (cl *Client) Routes(mux *http.ServeMux) {
	mux.Handle("/", templ.Handler(templates.Index()))
	mux.Handle("/templates/v1/login", templ.Handler(templates.Login()))
	mux.Handle("/templates/v1/text-stream", templ.Handler(templates.TextStream()))
	mux.HandleFunc("/templates/v1/text-stream/events", func(w http.ResponseWriter, r *http.Request) {
		// Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Example markdown content
		markdownLines := []string{
			"# Streaming Markdown\nThis is line **1** with some *italic* text.",
			"## Code Example\n```python\nprint('Hello from line 2!')\nprint('Hello from line 2!')\n```",
			"> This is line 3 with a blockquote\n\nAnd some regular text.",
			"* Line 4 is a list item\n* With multiple points\n* And formatting **bold**",
			"### Final Line\nLine 5 with a [link](https://example.com) and `inline code`",
		}

		// Send messages
		for _, markdown := range markdownLines {
			select {
			case <-r.Context().Done():
				return
			default:
				var buf bytes.Buffer
				if err := cl.md.Convert([]byte(markdown), &buf); err != nil {
					fmt.Fprintf(w, "event: text-update\n")
					writeSSELines(w, fmt.Sprintf("<div class=\"error\">Error rendering markdown: %s</div>", err))
					w.(http.Flusher).Flush()
					continue
				}

				fmt.Fprintf(w, "event: text-update\n")
				writeSSELines(w, buf.String())
				w.(http.Flusher).Flush()
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Send completion message
		var buf bytes.Buffer
		cl.md.Convert([]byte("## 🎉 Streaming Complete!\n*All markdown content has been delivered successfully.*"), &buf)
		fmt.Fprintf(w, "event: text-update\n")
		writeSSELines(w, buf.String())
		w.(http.Flusher).Flush()

		// Send the close event
		fmt.Fprintf(w, "event: close\ndata: close\n\n")
		w.(http.Flusher).Flush()
	})
	mux.Handle("/templates/v1/hello", templ.Handler(templates.Hello("Peyton")))
}
