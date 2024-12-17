package service

import (
	"context"
	"sync"

	"github.com/ditto-assistant/backend/cfg/secr"
)

type Context struct {
	Background context.Context
	ShutdownWG *sync.WaitGroup
	Secr       *secr.Client
}
