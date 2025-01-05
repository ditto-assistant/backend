package web

import (
	"net/http"

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
	mux.Handle("/templates/v1/hello", templ.Handler(templates.Hello("Peyton")))
}
