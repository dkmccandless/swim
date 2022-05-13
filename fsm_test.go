package swim

import (
	"testing"
)

func TestIsNews(t *testing.T) {
	s := &stateMachine{
		members: map[id]*profile{
			"abc": {incarnation: 0},
			"def": {incarnation: 0},
			"ghi": {incarnation: 1},
			"jkl": {incarnation: 1},
		},
		suspects:     map[id]int{"def": 0, "jkl": 0},
		removed:      map[id]bool{"xyz": true},
		seenUserMsgs: map[id]bool{"1a2b3c": true},
	}
	for _, tt := range []struct {
		m    *message
		want bool
	}{
		{nil, false},
		{&message{Type: alive, ID: "abc", Incarnation: 0}, false},
		{&message{Type: suspected, ID: "abc", Incarnation: 0}, true},
		{&message{Type: alive, ID: "abc", Incarnation: 1}, true},
		{&message{Type: suspected, ID: "abc", Incarnation: 1}, true},
		{&message{Type: alive, ID: "def", Incarnation: 0}, false},
		{&message{Type: suspected, ID: "def", Incarnation: 0}, false},
		{&message{Type: alive, ID: "def", Incarnation: 1}, true},
		{&message{Type: suspected, ID: "def", Incarnation: 1}, true},
		{&message{Type: alive, ID: "ghi", Incarnation: 0}, false},
		{&message{Type: suspected, ID: "ghi", Incarnation: 0}, false},
		{&message{Type: alive, ID: "ghi", Incarnation: 1}, false},
		{&message{Type: suspected, ID: "ghi", Incarnation: 1}, true},
		{&message{Type: alive, ID: "jkl", Incarnation: 0}, false},
		{&message{Type: suspected, ID: "jkl", Incarnation: 0}, false},
		{&message{Type: alive, ID: "jkl", Incarnation: 1}, false},
		{&message{Type: suspected, ID: "jkl", Incarnation: 1}, false},
		{&message{Type: alive, ID: "mno", Incarnation: 0}, true},
		{&message{Type: suspected, ID: "mno", Incarnation: 0}, true},
		{&message{Type: alive, ID: "mno", Incarnation: 1}, true},
		{&message{Type: suspected, ID: "mno", Incarnation: 1}, true},
		{&message{Type: alive, ID: "xyz", Incarnation: 0}, false},
		{&message{Type: suspected, ID: "xyz", Incarnation: 0}, false},
		{&message{Type: alive, ID: "xyz", Incarnation: 1}, false},
		{&message{Type: failed, ID: "abc"}, true},
		{&message{Type: failed, ID: "def"}, true},
		{&message{Type: failed, ID: "ghi"}, true},
		{&message{Type: failed, ID: "jkl"}, true},
		{&message{Type: failed, ID: "mno"}, true},
		{&message{Type: failed, ID: "xyz"}, false},
		{
			&message{
				Type: userMsg, ID: "abc", MessageID: "1a2b3c", Body: []byte{
					72, 101, 108, 108, 111, 44, 32, 119, 111, 114, 108, 100, 33,
				},
			},
			false,
		},
		{
			&message{
				Type: userMsg, ID: "abc", MessageID: "4d5e6f", Body: []byte{
					72, 101, 108, 108, 111, 44, 32, 119, 111, 114, 108, 100, 33,
				},
			},
			true,
		},
		{
			&message{
				Type: userMsg, ID: "xyz", MessageID: "4d5e6f", Body: []byte{
					72, 101, 108, 108, 111, 44, 32, 119, 111, 114, 108, 100, 33,
				},
			},
			false,
		},
	} {
		if got := s.isNews(tt.m); got != tt.want {
			t.Errorf("isNews(%+v): got %v, expected %v", tt.m, got, tt.want)
		}
	}
}
