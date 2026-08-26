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

func (c *assayViewCache) Put(view *AssayView) {
	if view == nil || view.Assay == nil {
		return
	}
	payload, err := json.Marshal(view)
	if err != nil {
		return
	}
	c.mu.Lock()
	c.entries[view.Assay.ID] = payload
	c.mu.Unlock()
}

func (s *Service) rememberView(view *AssayView) *AssayView {
	s.views.Put(view)
	return view
}
