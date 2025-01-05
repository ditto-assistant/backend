package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/ditto-assistant/backend/pkg/core"
	"github.com/ditto-assistant/backend/pkg/web/templates"
)

type Client struct {
	cl *core.Client
}

func NewClient(cl *core.Client) *Client {
	return &Client{cl: cl}
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

		// Send messages
		for i := 1; i <= 5; i++ {
			select {
			case <-r.Context().Done():
				// Client disconnected
				return
			default:
				// Send SSE message with styled div
				fmt.Fprintf(w, "event: text-update\n")
				fmt.Fprintf(w, "data: <div class=\"stream-line\">Streaming line %d via SSE...</div>\n\n", i)
				w.(http.Flusher).Flush()
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Send a final message
		fmt.Fprintf(w, "event: text-update\n")
		fmt.Fprintf(w, "data: <div class=\"stream-line\" style=\"color: green;\">Streaming complete!</div>\n\n")
		w.(http.Flusher).Flush()

		// Send the close event with data
		fmt.Fprintf(w, "event: close\n")
		fmt.Fprintf(w, "data: close\n\n")
		w.(http.Flusher).Flush()
	})
	mux.Handle("/templates/v1/hello", templ.Handler(templates.Hello("Peyton")))
}
