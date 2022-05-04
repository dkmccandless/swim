package roundrobinrandom

import (
	"reflect"
	"testing"
)

var addTests = []struct {
	o     *Order[string]
	value string
	wants []*Order[string]
}{
	{
		new(Order[string]),
		"a",
		[]*Order[string]{
			{[]string{"a"}, 0},
		},
	},
	{
		&Order[string]{[]string{"a"}, 0},
		"b",
		[]*Order[string]{
			{[]string{"b", "a"}, 0},
			{[]string{"a", "b"}, 0},
		},
	},
	{
		&Order[string]{[]string{"a"}, 1},
		"b",
		[]*Order[string]{
			{[]string{"a", "b"}, 2},
			{[]string{"a", "b"}, 1},
		},
	},
	{
		&Order[string]{[]string{"a", "b"}, 0},
		"c",
		[]*Order[string]{
			{[]string{"c", "b", "a"}, 0},
			{[]string{"a", "c", "b"}, 0},
			{[]string{"a", "b", "c"}, 0},
		},
	},
	{
		&Order[string]{[]string{"a", "b"}, 1},
		"c",
		[]*Order[string]{
			{[]string{"a", "c", "b"}, 2},
			{[]string{"a", "c", "b"}, 1},
			{[]string{"a", "b", "c"}, 1},
		},
	},
	{
		&Order[string]{[]string{"a", "b"}, 2},
		"c",
		[]*Order[string]{
			{[]string{"a", "b", "c"}, 3},
			{[]string{"a", "b", "c"}, 3},
			{[]string{"a", "b", "c"}, 2},
		},
	},
}

func TestAddAt(t *testing.T) {
	for _, tt := range addTests {
		for k, want := range tt.wants {
			got := clone(tt.o)
			got.addAt(tt.value, k)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%+v.addAt(%q, %v): %+v != %+v",
					tt.o, tt.value, k, got, want,
				)
			}
		}
	}
}

func TestNext(t *testing.T) {
	o := &Order[string]{[]string{"a", "b", "c"}, 0}
	for _, tt := range []struct {
		next string
		o    *Order[string]
	}{
		{"a", &Order[string]{[]string{"a", "b", "c"}, 1}},
		{"b", &Order[string]{[]string{"a", "b", "c"}, 2}},
		{"c", &Order[string]{[]string{"a", "b", "c"}, 3}},
	} {
		old := clone(o)
		got := o.Next()
		if got != tt.next {
			t.Errorf("%+v.Next(): %v != %v", old, got, tt.next)
		}
		if !reflect.DeepEqual(o, tt.o) {
			t.Errorf("%+v.Next(): %+v != %+v", old, o, tt.o)
		}
	}

	// Shuffle
	old := clone(o)
	o.Next()
	if next := 1; o.next != next {
		t.Errorf("%+v.Next() becomes %+v (expect next == %v", old, o, next)
	}
	if !reflect.DeepEqual(elemCount(o.a), elemCount(old.a)) {
		t.Errorf("%+v.Next(): %v is not a permutation of %v", old, o.a, old.a)
	}
}

var removeTests = []struct {
	o     *Order[string]
	wants []*Order[string]
}{
	{
		&Order[string]{[]string{"a"}, 0},
		[]*Order[string]{
			{[]string{}, 0},
		},
	},
	{
		&Order[string]{[]string{"a"}, 1},
		[]*Order[string]{
			{[]string{}, 0},
		},
	},
	{
		&Order[string]{[]string{"a", "b"}, 0},
		[]*Order[string]{
			{[]string{"b"}, 0},
			{[]string{"a"}, 0},
		},
	},
	{
		&Order[string]{[]string{"a", "b"}, 1},
		[]*Order[string]{
			{[]string{"b"}, 0},
			{[]string{"a"}, 1},
		},
	},
	{
		&Order[string]{[]string{"a", "b"}, 2},
		[]*Order[string]{
			{[]string{"b"}, 1},
			{[]string{"a"}, 1},
		},
	},
	{
		&Order[string]{[]string{"a", "b", "c", "d"}, 2},
		[]*Order[string]{
			{[]string{"b", "d", "c"}, 1},
			{[]string{"a", "d", "c"}, 1},
			{[]string{"a", "b", "d"}, 2},
			{[]string{"a", "b", "c"}, 2},
		},
	},
}

func TestRemoveAt(t *testing.T) {
	for _, tt := range removeTests {
		for k, want := range tt.wants {
			got := clone(tt.o)
			got.removeAt(k)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%+v.removeAt(%v): %+v != %+v",
					tt.o, k, got, want,
				)
			}
		}
	}
}

// clone returns a fresh copy of o.
func clone[T comparable](o *Order[T]) *Order[T] {
	return &Order[T]{append([]T{}, o.a...), o.next}
}

// elemCount returns the counts of a's elements.
func elemCount[T comparable](a []T) map[T]int {
	m := make(map[T]int)
	for _, v := range a {
		m[v]++
	}
	return m
}
