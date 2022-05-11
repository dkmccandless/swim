package rpq

import (
	"fmt"
	"reflect"
	"testing"
)

func TestPQSwap(t *testing.T) {
	for _, tt := range []struct {
		pq   priorityQueue[string, int]
		i, j int
		want priorityQueue[string, int]
	}{
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 6, 2},
					{"def", 1, 1},
					{"ghi", 4, 5},
				},
				map[string]int{"abc": 0, "def": 1, "ghi": 2},
			},
			0, 1,
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"def", 1, 1},
					{"abc", 6, 2},
					{"ghi", 4, 5},
				},
				map[string]int{"abc": 1, "def": 0, "ghi": 2},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 6, 2},
					{"def", 1, 1},
					{"ghi", 4, 5},
				},
				map[string]int{"def": 1, "ghi": 2},
			},
			0, 1,
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"def", 1, 1},
					{"", 6, 2},
					{"ghi", 4, 5},
				},
				map[string]int{"def": 0, "ghi": 2},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 6, 2},
					{"", 1, 1},
					{"ghi", 4, 5},
				},
				map[string]int{"ghi": 2},
			},
			0, 1,
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 1, 1},
					{"", 6, 2},
					{"ghi", 4, 5},
				},
				map[string]int{"ghi": 2},
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.pq)
		tt.pq.Swap(tt.i, tt.j)
		if !reflect.DeepEqual(tt.pq, tt.want) {
			t.Errorf("%v.Swap(%v, %v): got %+v, expected %+v",
				s, tt.i, tt.j, tt.pq, tt.want,
			)
		}
	}
}

func TestPQPush(t *testing.T) {
	for _, tt := range []struct {
		pq   priorityQueue[string, int]
		a    *item[string, int]
		want priorityQueue[string, int]
	}{
		{
			makePriorityQueue[string, int](),
			&item[string, int]{"", 2, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{{"", 2, 0}},
				map[string]int{},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 2, 0},
				},
				map[string]int{},
			},
			&item[string, int]{"", 2, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 2, 0},
					{"", 2, 0},
				},
				map[string]int{},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 2, 0},
				},
				map[string]int{},
			},
			&item[string, int]{"abc", 2, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 2, 0},
					{"abc", 2, 0},
				},
				map[string]int{"abc": 1},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
				},
				map[string]int{"abc": 0},
			},
			&item[string, int]{"", 2, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
					{"", 2, 0},
				},
				map[string]int{"abc": 0},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 1},
					{"def", 3, 2},
				},
				map[string]int{"abc": 0, "def": 1},
			},
			&item[string, int]{"ghi", 5, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 1},
					{"def", 3, 2},
					{"ghi", 5, 0},
				},
				map[string]int{"abc": 0, "def": 1, "ghi": 2},
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.pq)
		tt.pq.Push(tt.a)
		if !reflect.DeepEqual(tt.pq, tt.want) {
			t.Errorf("%v.Push(%+v): got %+v, expected %+v",
				s, tt.a, tt.pq, tt.want,
			)
		}
	}
}

func TestPQPop(t *testing.T) {
	for _, tt := range []struct {
		pq   priorityQueue[string, int]
		item *item[string, int]
		want priorityQueue[string, int]
	}{
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
				},
				map[string]int{"abc": 0},
			},
			&item[string, int]{"abc", 2, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{},
				map[string]int{},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
					{"def", 3, 2},
				},
				map[string]int{"abc": 0, "def": 1},
			},
			&item[string, int]{"def", 3, 2},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
				},
				map[string]int{"abc": 0},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
					{"", 3, 2},
					{"", 5, 0},
				},
				map[string]int{"abc": 0},
			},
			&item[string, int]{"", 5, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
					{"", 3, 2},
				},
				map[string]int{"abc": 0},
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.pq)
		item := tt.pq.Pop()
		if !reflect.DeepEqual(item, tt.item) ||
			!reflect.DeepEqual(tt.pq, tt.want) {
			t.Errorf("%v.Pop(): got %+v, %+v; expected %+v, %+v",
				s, item, tt.pq, tt.item, tt.want,
			)
		}
	}
}
