package application

import (
	"encoding/json"
	"sync"
)

type assayViewCache struct {
	mu      sync.RWMutex
	entries map[string][]byte
}

func newAssayViewCache() assayViewCache {
	return assayViewCache{entries: make(map[string][]byte)}
}

func (c *assayViewCache) Get(id string) (*AssayView, bool) {
	c.mu.RLock()
	payload, ok := c.entries[id]
	copyOfPayload := append([]byte(nil), payload...)
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	var view AssayView
	if err := json.Unmarshal(copyOfPayload, &view); err != nil {
		return nil, false
	}
	return &view, true
}

// Put stores the view unless a newer revision is already cached. Writes commit
// under the repository's serialized update lock and therefore always carry the
// latest revision, so they never lose to a stale concurrent reader. A read that
// observed an older revision must not clobber a newer cached entry.
func (c *assayViewCache) Put(view *AssayView) {
	if view == nil || view.Assay == nil {
		return
	}
	payload, err := json.Marshal(view)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[view.Assay.ID]; ok {
		var cached AssayView
		if err := json.Unmarshal(existing, &cached); err == nil && cached.Assay != nil && cached.Assay.Revision > view.Assay.Revision {
			return
		}
	}
	c.entries[view.Assay.ID] = payload
}

func (s *Service) rememberView(view *AssayView) *AssayView {
	s.views.Put(view)
	return view
}
