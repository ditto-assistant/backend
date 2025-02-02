package ty

import (
	"context"
	"sync"
	"time"
)

type Result[T any] struct {
	Ok  T
	Err error
}

type ShutdownContext struct {
	Background       context.Context
	WaitGroup        *sync.WaitGroup
	ShutdownDuration time.Duration
}

func (s ShutdownContext) Run(f func(ctx context.Context)) {
	s.WaitGroup.Add(1)
	go func() {
		ctx, cancel := context.WithTimeout(s.Background, s.ShutdownDuration)
		defer func() {
			cancel()
			s.WaitGroup.Done()
		}()
		f(ctx)
	}()
}
