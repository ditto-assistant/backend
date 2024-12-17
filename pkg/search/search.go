package search

import (
	"context"
	"io"
	"log/slog"

	"github.com/ditto-assistant/backend/pkg/db/users"
	"github.com/ditto-assistant/backend/pkg/service"
)

type Service interface {
	Search(ctx context.Context, req Request) (results Results, err error)
}

type Request struct {
	User       users.User
	Query      string
	NumResults int
}

type Results interface {
	Text(w io.Writer) error
}

type Client struct {
	sc       service.Context
	services []Service
}

type Option func(*Client)

func WithService(svc Service) Option {
	return func(c *Client) {
		c.services = append(c.services, svc)
	}
}

func NewClient(opts ...Option) *Client {
	c := &Client{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

const MAX_TRIES = 4

func (c *Client) Search(ctx context.Context, req Request) (results Results, err error) {
	for i := range MAX_TRIES {
		i = i % len(c.services)
		svc := c.services[i]
		results, err = svc.Search(ctx, req)
		if err == nil {
			return
		}
		slog.Warn("Retrying search with next service", "error", err, "try", i+1)
	}
	slog.Error("Failed to search with all services", "error", err)
	return
}
