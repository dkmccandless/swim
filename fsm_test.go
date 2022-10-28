package swim

import (
	"reflect"
	"testing"
)

func TestIsMemberNews(t *testing.T) {
	s := &stateMachine{
		members: map[id]*profile{
			"abc": {incarnation: 0},
			"def": {incarnation: 0},
			"ghi": {incarnation: 1},
			"jkl": {incarnation: 1},
		},
		suspects: map[id]int{"def": 0, "jkl": 0},
		removed:  map[id]bool{"xyz": true},
	}
	for _, tt := range []struct {
		m    *message
		want bool
	}{
		{nil, false},
		{&message{Type: alive, NodeID: "abc", Incarnation: 0}, false},
		{&message{Type: suspected, NodeID: "abc", Incarnation: 0}, true},
		{&message{Type: alive, NodeID: "abc", Incarnation: 1}, true},
		{&message{Type: suspected, NodeID: "abc", Incarnation: 1}, true},
		{&message{Type: alive, NodeID: "def", Incarnation: 0}, false},
		{&message{Type: suspected, NodeID: "def", Incarnation: 0}, false},
		{&message{Type: alive, NodeID: "def", Incarnation: 1}, true},
		{&message{Type: suspected, NodeID: "def", Incarnation: 1}, true},
		{&message{Type: alive, NodeID: "ghi", Incarnation: 0}, false},
		{&message{Type: suspected, NodeID: "ghi", Incarnation: 0}, false},
		{&message{Type: alive, NodeID: "ghi", Incarnation: 1}, false},
		{&message{Type: suspected, NodeID: "ghi", Incarnation: 1}, true},
		{&message{Type: alive, NodeID: "jkl", Incarnation: 0}, false},
		{&message{Type: suspected, NodeID: "jkl", Incarnation: 0}, false},
		{&message{Type: alive, NodeID: "jkl", Incarnation: 1}, false},
		{&message{Type: suspected, NodeID: "jkl", Incarnation: 1}, false},
		{&message{Type: alive, NodeID: "mno", Incarnation: 0}, true},
		{&message{Type: suspected, NodeID: "mno", Incarnation: 0}, true},
		{&message{Type: alive, NodeID: "mno", Incarnation: 1}, true},
		{&message{Type: suspected, NodeID: "mno", Incarnation: 1}, true},
		{&message{Type: alive, NodeID: "xyz", Incarnation: 0}, false},
		{&message{Type: suspected, NodeID: "xyz", Incarnation: 0}, false},
		{&message{Type: alive, NodeID: "xyz", Incarnation: 1}, false},
		{&message{Type: failed, NodeID: "abc"}, true},
		{&message{Type: failed, NodeID: "def"}, true},
		{&message{Type: failed, NodeID: "ghi"}, true},
		{&message{Type: failed, NodeID: "jkl"}, true},
		{&message{Type: failed, NodeID: "mno"}, true},
		{&message{Type: failed, NodeID: "xyz"}, false},
	} {
		if got := s.isMemberNews(tt.m); got != tt.want {
			t.Errorf("isNews(%+v): got %v, expected %v", tt.m, got, tt.want)
		}
	}
}

func TestStripMemo(t *testing.T) {
	for _, tt := range []struct {
		in, want *message
	}{
		{
			&message{Type: alive, NodeID: "abc", Incarnation: 2},
			&message{Type: alive, NodeID: "abc", Incarnation: 2},
		},
		{
			&message{
				Type:        alive,
				NodeID:      "abc",
				Incarnation: 2,
				MemoID:      "123",
				Body:        []byte("Hello, SWIM!"),
			},
			&message{Type: alive, NodeID: "abc", Incarnation: 2},
		},
	} {
		m := new(message)
		*m = *tt.in
		got := stripMemo(m)
		if !reflect.DeepEqual(m, tt.in) {
			t.Errorf("stripMemo(%v): input modified to %v", tt.in, m)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("stripMemo(%v): got %v, expect %v", tt.in, got, tt.want)
		}
	}
}
