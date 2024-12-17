package ty

import (
	"context"
	"sync"
)

type Result[T any] struct {
	Ok  T
	Err error
}

type ServiceContext struct {
	Background context.Context
	ShutdownWG *sync.WaitGroup
}
