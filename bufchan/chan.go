// Package bufchan implements a channel with a dynamically sized buffer.
package bufchan

// A Chan is a channel with a buffer of variable length. A send operation on a
// Chan always proceeds immediately. A receive operation blocks if the buffer
// is empty.
//
// A Chan may be closed by closing its send-only endpoint. A closed Chan
// behaves identically to a closed buffered channel: closing or sending on it
// causes a run-time panic, and receiving from it proceeds immediately,
// yielding the type argument's zero value after any previously sent values
// are received. A multi-valued receive operation indicates whether the Chan
// is closed and empty.
type Chan[T any] struct {
	in  chan<- T
	out <-chan T
}

// Make returns a new Chan.
func Make[T any]() Chan[T] {
	in := make(chan T)
	out := make(chan T)
	go func() {
		defer close(out)
		var buf []T
		for {
			if len(buf) == 0 {
				t, ok := <-in
				if !ok {
					break
				}
				buf = append(buf, t)
			}
			select {
			case t, ok := <-in:
				if !ok {
					break
				}
				buf = append(buf, t)
			case out <- buf[0]:
				buf = buf[1:]
			}
		}
		for len(buf) != 0 {
			out <- buf[0]
			buf = buf[1:]
		}
	}()
	return Chan[T]{in, out}
}

// Send returns the Chan's send-only endpoint.
func (c Chan[T]) Send() chan<- T {
	return c.in
}

// Receive returns the Chan's receive-only endpoint.
func (c Chan[T]) Receive() <-chan T {
	return c.out
}
