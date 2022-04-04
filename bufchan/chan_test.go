package bufchan

import (
	"testing"

	"kr.dev/diff"
)

func TestChan(t *testing.T) {
	ch := Make[int]()
	go func() {
		ch.Send() <- 1
		ch.Send() <- 2
		ch.Send() <- 3
		close(ch.Send())
	}()

	diff.Test(t, t.Errorf, <-ch.Receive(), 1)
	diff.Test(t, t.Errorf, <-ch.Receive(), 2)
	diff.Test(t, t.Errorf, <-ch.Receive(), 3)

	n, ok := <-ch.Receive()
	diff.Test(t, t.Errorf, n, 0)
	diff.Test(t, t.Errorf, ok, false)
}
