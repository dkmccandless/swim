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
				map[string]int{"": 0, "def": 1, "ghi": 2},
			},
			0, 1,
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"def", 1, 1},
					{"", 6, 2},
					{"ghi", 4, 5},
				},
				map[string]int{"def": 0, "": 1, "ghi": 2},
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.pq)
		tt.pq.Swap(tt.i, tt.j)
		if !reflect.DeepEqual(tt.pq.toMap(), tt.want.toMap()) {
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
				map[string]int{"": 0},
			},
		},
		{
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 2, 0},
				},
				map[string]int{"": 0},
			},
			&item[string, int]{"abc", 4, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"", 2, 0},
					{"abc", 4, 0},
				},
				map[string]int{"": 0, "abc": 1},
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
		if !reflect.DeepEqual(tt.pq.toMap(), tt.want.toMap()) {
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
					{"def", 3, 2},
					{"ghi", 5, 0},
				},
				map[string]int{"abc": 0, "def": 1, "ghi": 2},
			},
			&item[string, int]{"ghi", 5, 0},
			priorityQueue[string, int]{
				[]*item[string, int]{
					{"abc", 2, 0},
					{"def", 3, 2},
				},
				map[string]int{"abc": 0, "def": 1},
			},
		},
	} {
		s := fmt.Sprintf("%+v", tt.pq)
		item := tt.pq.Pop()
		if !reflect.DeepEqual(item, tt.item) ||
			!reflect.DeepEqual(tt.pq.toMap(), tt.want.toMap()) {
			t.Errorf("%v.Pop(): got %+v, %+v; expected %+v, %+v",
				s, item, tt.pq, tt.item, tt.want,
			)
		}
	}
}

type valuecount[V any] struct {
	value V
	count int
}

// toMap returns a map that describes pq's contents, up to ordering.
func (pq priorityQueue[K, V]) toMap() map[K]valuecount[V] {
	m := make(map[K]valuecount[V])
	for _, item := range pq.items {
		m[item.key] = valuecount[V]{item.value, item.count}
	}
	return m
}
