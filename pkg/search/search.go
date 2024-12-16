package search

import (
	"context"
	"io"
	"log/slog"
)

type Service interface {
	Search(ctx context.Context, req Request) (results Results, err error)
}

type Request struct {
	Query      string
	NumResults int
}

type Results interface {
	Text(w io.Writer) error
}

type Client struct {
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

func (c *Client) Search(ctx context.Context, req Request) (results Results, err error) {
	for _, svc := range c.services {
		results, err = svc.Search(ctx, req)
		if err == nil {
			return
		}
		slog.Warn("Retrying search with next service", "error", err)
	}
	return
}
