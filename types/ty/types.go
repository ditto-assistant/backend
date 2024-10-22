package ty

type Result[T any] struct {
	Ok  T
	Err error
}
