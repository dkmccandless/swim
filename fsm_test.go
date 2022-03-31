package swim

import (
	"testing"

	"kr.dev/diff"
)

func TestSupersedes(t *testing.T) {
	a1 := &message{alive, "abc", 1}
	s1 := &message{suspected, "abc", 1}
	a2 := &message{alive, "abc", 2}
	s2 := &message{suspected, "abc", 2}
	f := &message{failed, "abc", 0}
	for _, test := range []struct {
		a, b *message
		want bool
	}{
		{nil, nil, false},
		{nil, f, false},
		{nil, a1, false},
		{nil, s1, false},
		{f, nil, true},
		{a1, nil, true},
		{s1, nil, true},
		{a1, f, false},
		{s1, f, false},
		{f, f, false},
		{f, a1, true},
		{f, s1, true},
		{a1, a1, false},
		{a1, a2, false},
		{a2, a1, true},
		{s1, s1, false},
		{s1, s2, false},
		{s2, s1, true},
		{a1, s1, false},
		{s1, a1, true},
		{s1, a2, false},
		{a2, s1, true},
		{a1, s2, false},
		{s2, a1, true},
		{a2, &message{alive, "def", 1}, false},
		{&message{alive, "def", 2}, a1, false},
	} {
		diff.Test(t, t.Errorf, supersedes(test.a, test.b), test.want)
	}
}
