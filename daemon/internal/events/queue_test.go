package events

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// TestQueueMatchesASlice runs random pushes and pops on both a queue and a plain slice and checks
// that they always agree, through many wraps and growths.
func TestQueueMatchesASlice(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var q queue
	var model []uint64
	var next uint64
	for step := range 5000 {
		if rng.IntN(3) > 0 || len(model) == 0 {
			next++
			q.push(Event{Seq: next})
			model = append(model, next)
		} else {
			got, ok := q.pop()
			if !ok || got.Seq != model[0] {
				t.Fatalf("step %d: pop = %d, %v, want %d", step, got.Seq, ok, model[0])
			}
			model = model[1:]
		}
		if q.len() != len(model) {
			t.Fatalf("step %d: len = %d, want %d", step, q.len(), len(model))
		}
		if head, ok := q.peek(); ok != (len(model) > 0) || (ok && head.Seq != model[0]) {
			t.Fatalf("step %d: peek = %v, %v", step, head, ok)
		}
	}
	all := make([]uint64, 0, q.len())
	for i := range q.len() {
		all = append(all, q.at(i).Seq)
	}
	if !slices.Equal(all, model) {
		t.Errorf("at(i) reads %v, want %v", all, model)
	}
	q.clear()
	if _, ok := q.pop(); ok || q.len() != 0 {
		t.Error("a cleared queue should be empty")
	}
}
