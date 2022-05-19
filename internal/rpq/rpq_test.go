package rpq

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestPush(t *testing.T) {
	five := func() int { return 5 }
	for _, tt := range []struct {
		q     *Queue[string, int]
		key   string
		value int
		want  *Queue[string, int]
	}{
		{
			New[string, int](five),
			"", 2,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 2, 0},
					},
					map[string]int{"": 0},
				},
				five,
			},
		},
		{
			New[string, int](five),
			"abc", 2,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 2, 0},
					},
					map[string]int{"abc": 0},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 2, 1},
					},
					map[string]int{"": 0},
				},
				five,
			},
			"abc", 2,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 2, 0},
						{"", 2, 1},
					},
					map[string]int{"abc": 0, "": 1},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 2, 1},
					},
					map[string]int{"abc": 0},
				},
				five,
			},
			"", 2,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 2, 0},
						{"abc", 2, 1},
					},
					map[string]int{"": 0, "abc": 1},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 2, 1},
						{"def", 3, 2},
					},
					map[string]int{"": 0, "def": 1},
				},
				five,
			},
			"abc", 2,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 2, 0},
						{"", 2, 1},
						{"def", 3, 2},
					},
					map[string]int{"abc": 0, "": 1, "def": 2},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 2, 1},
						{"abc", 3, 2},
						{"def", 3, 3},
					},
					map[string]int{"": 0, "abc": 1, "def": 2},
				},
				five,
			},
			"abc", 5,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 5, 0},
						{"", 2, 1},
						{"def", 3, 3},
					},
					map[string]int{"abc": 0, "": 1, "def": 2},
				},
				five,
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.q)
		tt.q.Push(tt.key, tt.value)
		if !reflect.DeepEqual(tt.q.pq.toMap(), tt.want.pq.toMap()) {
			t.Errorf("%v.Push(%q, %v): got %+v, expected %+v",
				s, tt.key, tt.value, tt.q, tt.want,
			)
		}
	}
}

func TestPop(t *testing.T) {
	five := func() int { return 5 }
	for _, tt := range []struct {
		q     *Queue[string, int]
		value int
		want  *Queue[string, int]
	}{
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 6, 0},
						{"def", 2, 2},
						{"ghi", 0, 4},
					},
					map[string]int{"abc": 0, "def": 1, "ghi": 2},
				},
				five,
			},
			6,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 6, 1},
						{"def", 2, 2},
						{"ghi", 0, 4},
					},
					map[string]int{"abc": 0, "def": 1, "ghi": 2},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 6, 2},
						{"def", 2, 2},
						{"ghi", 0, 4},
					},
					map[string]int{"abc": 0, "def": 1, "ghi": 2},
				},
				five,
			},
			6,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"def", 2, 2},
						{"abc", 6, 3},
						{"ghi", 0, 4},
					},
					map[string]int{"abc": 1, "def": 0, "ghi": 2},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 6, 4},
						{"def", 2, 4},
						{"ghi", 0, 5},
					},
					map[string]int{"abc": 0, "def": 1, "ghi": 2},
				},
				five,
			},
			6,
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"def", 2, 4},
						{"ghi", 0, 5},
					},
					map[string]int{"def": 0, "ghi": 1},
				},
				five,
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.q)
		value := tt.q.Pop()
		if !reflect.DeepEqual(value, tt.value) ||
			!reflect.DeepEqual(tt.q.pq.toMap(), tt.want.pq.toMap()) {
			t.Errorf("%v.Pop(): got %+v, %+v; expected %+v, %+v",
				s, value, tt.q, tt.value, tt.want,
			)
		}
	}
}

func TestPopN(t *testing.T) {
	five := func() int { return 5 }
	for _, tt := range []struct {
		q      *Queue[string, int]
		n      int
		values []int
		want   *Queue[string, int]
	}{
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 1, 0},
						{"abc", 2, 2},
						{"def", 3, 3},
					},
					map[string]int{"": 0, "abc": 1, "def": 2},
				},
				five,
			},
			4,
			[]int{1, 2, 3},
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 1, 1},
						{"abc", 2, 3},
						{"def", 3, 4},
					},
					map[string]int{"": 0, "abc": 1, "def": 2},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"a", 1, 1},
						{"b", 2, 1},
						{"c", 3, 1},
						{"d", 4, 1},
						{"e", 0, 3},
						{"f", 0, 3},
					},
					map[string]int{
						"a": 0, "b": 1, "c": 2, "d": 3, "e": 4, "f": 5,
					},
				},
				five,
			},
			4,
			[]int{1, 2, 3, 4},
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"a", 1, 2},
						{"b", 2, 2},
						{"c", 3, 2},
						{"d", 4, 2},
						{"e", 0, 3},
						{"f", 0, 3},
					},
					map[string]int{
						"a": 0, "b": 1, "c": 2, "d": 3, "e": 4, "f": 5,
					},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"a", 1, 3},
						{"b", 2, 4},
						{"c", 3, 4},
					},
					map[string]int{"a": 0, "b": 1, "c": 2},
				},
				five,
			},
			4,
			[]int{1, 2, 3},
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"a", 1, 4},
					},
					map[string]int{"a": 0},
				},
				five,
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.q)
		values := tt.q.PopN(tt.n)
		sort.Ints(values)
		if !reflect.DeepEqual(values, tt.values) ||
			!reflect.DeepEqual(tt.q.pq.toMap(), tt.want.pq.toMap()) {
			t.Errorf("%v.PopN(): got %+v, %+v; expected %+v, %+v",
				s, values, tt.q, tt.values, tt.want,
			)
		}
	}
}

func TestRemove(t *testing.T) {
	five := func() int { return 5 }
	for _, tt := range []struct {
		q    *Queue[string, int]
		key  string
		want *Queue[string, int]
	}{
		{
			New[string, int](five),
			"abc",
			New[string, int](five),
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 2, 0},
						{"def", 3, 1},
					},
					map[string]int{"def": 1},
				},
				five,
			},
			"abc",
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"", 2, 0},
						{"def", 3, 1},
					},
					map[string]int{"def": 1},
				},
				five,
			},
		},
		{
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"abc", 2, 0},
						{"def", 3, 1},
					},
					map[string]int{"abc": 0, "def": 1},
				},
				five,
			},
			"abc",
			&Queue[string, int]{
				priorityQueue[string, int]{
					[]*item[string, int]{
						{"def", 3, 1},
					},
					map[string]int{"def": 0},
				},
				five,
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.q)
		tt.q.Remove(tt.key)
		if !reflect.DeepEqual(tt.q.pq.toMap(), tt.want.pq.toMap()) {
			t.Errorf("%v.Remove(%q): got %+v, expected %+v",
				s, tt.key, tt.q, tt.want,
			)
		}
	}
}
