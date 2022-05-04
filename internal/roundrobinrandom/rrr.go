// Package roundrobinrandom implements a randomized round-based ordering type.
//
// Each element in an Order is visited in random sequence, once per round
// (possibly excluding the round it is added to the Order). Therefore, in an
// Order of n elements, successive visits to the same element will occur at
// intervals of at most 2n-1 calls to Next.
package roundrobinrandom

import "math/rand"

// An Order holds values to return in a randomized round-based sequence such
// that each value is returned once per round. The sequence is shuffled after
// each round. The zero value of type Order is an empty Order ready for use.
type Order[T comparable] struct {
	a    []T
	next int
}

// Next returns the next value in the Order, shuffling first if necessary. If
// the Order is empty, it returns the zero value of type T.
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

// Remove removes the first instance of t from the Order, if any.
func (o *Order[T]) Remove(t T) {
	for i := range o.a {
		if o.a[i] == t {
			o.removeAt(i)
			return
		}
	}
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

// IndependentSample returns a slice of unique elements besides exclude, chosen
// at random. If there are at least n such elements, IndependentSample returns
// n of them, or else all of them.
func (o *Order[T]) IndependentSample(n int, exclude T) []T {
	var ts []T
	for _, i := range rand.Perm(len(o.a)) {
		t := o.a[i]
		if t == exclude {
			continue
		}
		ts = append(ts, t)
		if len(ts) == n {
			break
		}
	}
	return ts
}

func (o *Order[T]) swap(i, j int) {
	o.a[i], o.a[j] = o.a[j], o.a[i]
}
