package ty

import (
	"context"
	"sync"
)

type Result[T any] struct {
	Ok  T
	Err error
}

type ShutdownContext struct {
	Background context.Context
	WaitGroup  *sync.WaitGroup
}
