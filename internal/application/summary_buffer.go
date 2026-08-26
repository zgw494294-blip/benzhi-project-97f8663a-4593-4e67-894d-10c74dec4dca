package application

import (
	"sync"

	"seed-vigor-workbench/internal/domain"
)

// assaySummaryBuffer reuses the projection allocation because list responses
// tend to contain the same number of assays across adjacent refreshes.
type assaySummaryBuffer struct {
	mu    sync.Mutex
	items []*AssaySummary
}

func (b *assaySummaryBuffer) Project(assays []domain.GerminationAssay) []*AssaySummary {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cap(b.items) < len(assays) {
		b.items = make([]*AssaySummary, len(assays))
	} else {
		b.items = b.items[:len(assays)]
	}
	for index := range assays {
		if b.items[index] == nil {
			b.items[index] = &AssaySummary{}
		}
		*b.items[index] = BuildSummary(&assays[index])
	}
	return b.items
}
