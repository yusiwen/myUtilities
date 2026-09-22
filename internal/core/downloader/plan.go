package downloader

import (
	"sort"
	"sync"
)

// plan splits the remote file into fixed-size blocks and tracks which ones are
// already present. Workers pull the next unfinished block, so a slow connection
// only delays a single block instead of holding back a whole contiguous chunk.
type plan struct {
	mu        sync.Mutex
	size      int64
	blockSize int64
	blocks    int64
	done      []bool
	doneCount int64
	cursor    int64
}

// newPlan builds a plan for size bytes, marking doneBlocks as already present.
// Out-of-range or duplicate block indexes are ignored.
func newPlan(size, blockSize int64, doneBlocks []int64) *plan {
	blocks := int64(0)
	if size > 0 && blockSize > 0 {
		blocks = (size + blockSize - 1) / blockSize
	}
	p := &plan{size: size, blockSize: blockSize, blocks: blocks, done: make([]bool, blocks)}
	for _, i := range doneBlocks {
		if i >= 0 && i < blocks && !p.done[i] {
			p.done[i] = true
			p.doneCount++
		}
	}
	return p
}

// next returns the next unfinished block index, or false when none are left.
func (p *plan) next() (int64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.cursor < p.blocks {
		i := p.cursor
		p.cursor++
		if !p.done[i] {
			return i, true
		}
	}
	return 0, false
}

// mark records a block as successfully written.
func (p *plan) mark(i int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= p.blocks || p.done[i] {
		return
	}
	p.done[i] = true
	p.doneCount++
}

// blockRange returns the inclusive byte range [start, end] of a block.
func (p *plan) blockRange(i int64) (int64, int64) {
	start := i * p.blockSize
	end := start + p.blockSize - 1
	if end > p.size-1 {
		end = p.size - 1
	}
	return start, end
}

// blockBytes returns the number of payload bytes a block holds (the final block
// may be shorter than blockSize).
func (p *plan) blockBytes(i int64) int64 {
	start := i * p.blockSize
	remaining := p.size - start
	if remaining <= 0 {
		return 0
	}
	if remaining < p.blockSize {
		return remaining
	}
	return p.blockSize
}

// doneBytes is the number of payload bytes already present on disk.
func (p *plan) doneBytes() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	var total int64
	for i, ok := range p.done {
		if ok {
			total += p.blockBytes(int64(i))
		}
	}
	return total
}

// progress reports completed and total block counts.
func (p *plan) progress() (done, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.doneCount, p.blocks
}

// doneList returns the sorted list of completed block indexes.
func (p *plan) doneList() []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := make([]int64, 0, p.doneCount)
	for i, ok := range p.done {
		if ok {
			list = append(list, int64(i))
		}
	}
	sort.Slice(list, func(a, b int) bool { return list[a] < list[b] })
	return list
}
