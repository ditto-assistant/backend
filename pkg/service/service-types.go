package service

import (
	"context"
	"sync"

	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/core"
)

type Context struct {
	Background context.Context
	ShutdownWG *sync.WaitGroup
	Secr       *secr.Client
	App        *core.Service
}
