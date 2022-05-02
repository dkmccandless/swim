package order

import "math/rand"

// An Order holds values to return in a randomized round-based sequence such
// that each value is returned once per round. The sequence is shuffled after
// each round. The zero value of type Order is an empty Order ready for use.
type Order[T comparable] struct {
	a    []T
	next int
}

// Next returns the next value in the Order, shuffling first if necessary. It
// returns the zero value of type T if the Order is empty.
func (o *Order[T]) Next() T {
	var t T
	if len(o.a) == 0 {
		return t
	}
	if o.next == len(o.a) {
		o.next = 0
		rand.Shuffle(len(o.a), o.swap)
	}
	t = o.a[o.next]
	o.next++
	return t
}

// Add inserts t into a random position in the Order. Depending on where it is
// inserted, t may or may not be returned in the current round.
func (o *Order[T]) Add(t T) {
	o.addAt(t, rand.Intn(len(o.a)+1))
}

// addAt inserts t at index k, which must be in the range [0, len(o.a)].
func (o *Order[T]) addAt(t T, k int) {
	o.a = append(o.a, t)
	last := len(o.a) - 1
	if k < o.next {
		o.swap(o.next, last)
		o.next++
	} else {
		o.swap(k, last)
	}
}

// Remove removes the first instance of t from the Order, if any, and reports
// whether the Order contained t.
func (o *Order[T]) Remove(t T) bool {
	for i := range o.a {
		if o.a[i] == t {
			o.removeAt(i)
			return true
		}
	}
	return false
}

// removeAt removes the element at index k.
func (o *Order[T]) removeAt(k int) {
	if k < o.next {
		o.next--
		o.swap(k, o.next)
		k = o.next
	}
	last := len(o.a) - 1
	o.swap(k, last)
	o.a = o.a[:last]
}

func (o *Order[T]) swap(i, j int) {
	o.a[i], o.a[j] = o.a[j], o.a[i]
}
