package domain

import "time"

type AuditEvent struct {
	ID        string         `json:"id"`
	AssayID   string         `json:"assay_id"`
	Revision  int64          `json:"revision"`
	Action    string         `json:"action"`
	Actor     string         `json:"actor"`
	Details   map[string]any `json:"details,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
