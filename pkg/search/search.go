package search

import (
	"context"
	"io"
	"log/slog"

	"github.com/ditto-assistant/backend/pkg/db/users"
	"github.com/ditto-assistant/backend/types/ty"
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
	sc       ty.ServiceContext
	services []Service
}

type Option func(*Client) error

func WithService(svc func(ty.ServiceContext) (Service, error)) Option {
	return func(c *Client) error {
		svc, err := svc(c.sc)
		if err != nil {
			return err
		}
		c.services = append(c.services, svc)
		return nil
	}
}

func NewClient(sc ty.ServiceContext, opts ...Option) (*Client, error) {
	c := &Client{sc: sc}
	for _, opt := range opts {
		err := opt(c)
		if err != nil {
			return nil, err
		}
	}
	return c, nil
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
