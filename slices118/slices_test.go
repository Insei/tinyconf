package slices118

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

func TestIndex(t *testing.T) {
	items := []string{"a", "b", "c"}
	if got := Index(items, "b"); got != 1 {
		t.Errorf("Index = %d, want 1", got)
	}
	if got := Index(items, "x"); got != -1 {
		t.Errorf("Index = %d, want -1", got)
	}
	if !Contains(items, "c") || Contains(items, "x") {
		t.Error("Contains returns incorrect result")
	}
	isB := func(s string) bool { return s == "b" }
	isX := func(s string) bool { return s == "x" }
	if got := IndexFunc(items, isB); got != 1 {
		t.Errorf("IndexFunc = %d, want 1", got)
	}
	if got := IndexFunc(items, isX); got != -1 {
		t.Errorf("IndexFunc = %d, want -1", got)
	}
	if !ContainsFunc(items, isB) || ContainsFunc(items, isX) {
		t.Error("ContainsFunc returns incorrect result")
	}
}

func TestClone(t *testing.T) {
	if Clone([]int(nil)) != nil {
		t.Error("Clone of nil should be nil")
	}
	items := []int{1, 2, 3}
	cloned := Clone(items)
	cloned[0] = 10
	if !reflect.DeepEqual(items, []int{1, 2, 3}) || !reflect.DeepEqual(cloned, []int{10, 2, 3}) {
		t.Errorf("Clone should copy items: %v %v", items, cloned)
	}
}

type sortItem struct {
	key, order int
}

func TestSortStableFunc(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	cmp := func(a, b sortItem) int { return a.key - b.key }
	for _, n := range []int{0, 1, 2, 19, 20, 21, 50, 100, 257, 1000} {
		for _, keys := range []int{1, 3, 10, n + 1} {
			items := make([]sortItem, n)
			for i := range items {
				items[i] = sortItem{key: rnd.Intn(keys), order: i}
			}
			want := Clone(items)
			sort.SliceStable(want, func(i, j int) bool { return want[i].key < want[j].key })

			SortStableFunc(items, cmp)
			if !reflect.DeepEqual(want, items) {
				t.Fatalf("SortStableFunc returns incorrect order for %d items with %d keys", n, keys)
			}
		}
	}
	// sorted and reversed inputs
	for _, n := range []int{30, 64, 500} {
		items := make([]sortItem, n)
		for i := range items {
			items[i] = sortItem{key: n - i, order: i}
		}
		SortStableFunc(items, cmp)
		SortStableFunc(items, cmp)
		for i := 1; i < n; i++ {
			if items[i-1].key > items[i].key {
				t.Fatalf("SortStableFunc returns not sorted items for %d reversed items", n)
			}
		}
	}
}
