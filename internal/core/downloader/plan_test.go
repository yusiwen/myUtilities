package downloader

import (
	"reflect"
	"testing"
)

func TestNewPlanLayout(t *testing.T) {
	p := newPlan(10, 4, nil)
	if p.blocks != 3 {
		t.Fatalf("blocks = %d, want 3", p.blocks)
	}
	cases := []struct {
		idx        int64
		start, end int64
		bytes      int64
	}{
		{0, 0, 3, 4},
		{1, 4, 7, 4},
		{2, 8, 9, 2},
	}
	for _, c := range cases {
		start, end := p.blockRange(c.idx)
		if start != c.start || end != c.end {
			t.Errorf("blockRange(%d) = (%d, %d), want (%d, %d)", c.idx, start, end, c.start, c.end)
		}
		if got := p.blockBytes(c.idx); got != c.bytes {
			t.Errorf("blockBytes(%d) = %d, want %d", c.idx, got, c.bytes)
		}
	}
	if got := p.doneBytes(); got != 0 {
		t.Errorf("doneBytes = %d, want 0", got)
	}
}

func TestPlanNextAndMark(t *testing.T) {
	p := newPlan(16, 4, []int64{1, 3})

	var order []int64
	for {
		idx, ok := p.next()
		if !ok {
			break
		}
		order = append(order, idx)
		p.mark(idx)
	}
	if want := []int64{0, 2}; !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	if done, total := p.progress(); done != 4 || total != 4 {
		t.Errorf("progress = %d/%d, want 4/4", done, total)
	}
	if got := p.doneBytes(); got != 16 {
		t.Errorf("doneBytes = %d, want 16", got)
	}
	if _, ok := p.next(); ok {
		t.Error("next returned a block after the plan was exhausted")
	}
}

func TestPlanDoneBytesPartial(t *testing.T) {
	p := newPlan(10, 4, nil)
	p.mark(2)
	if got := p.doneBytes(); got != 2 {
		t.Errorf("doneBytes = %d, want 2 (final short block)", got)
	}
	if done, total := p.progress(); done != 1 || total != 3 {
		t.Errorf("progress = %d/%d, want 1/3", done, total)
	}
	if got := p.doneList(); !reflect.DeepEqual(got, []int64{2}) {
		t.Errorf("doneList = %v, want [2]", got)
	}
}

func TestPlanIgnoresInvalidDoneBlocks(t *testing.T) {
	p := newPlan(8, 4, []int64{-1, 0, 0, 5, 1})
	if p.doneCount != 2 {
		t.Errorf("doneCount = %d, want 2", p.doneCount)
	}
	if got := p.doneList(); !reflect.DeepEqual(got, []int64{0, 1}) {
		t.Errorf("doneList = %v, want [0 1]", got)
	}
}

func TestPlanZeroSize(t *testing.T) {
	p := newPlan(0, 1024, nil)
	if p.blocks != 0 {
		t.Errorf("blocks = %d, want 0", p.blocks)
	}
	if _, ok := p.next(); ok {
		t.Error("next returned a block for a zero-byte file")
	}
	if got := p.doneBytes(); got != 0 {
		t.Errorf("doneBytes = %d, want 0", got)
	}
}
