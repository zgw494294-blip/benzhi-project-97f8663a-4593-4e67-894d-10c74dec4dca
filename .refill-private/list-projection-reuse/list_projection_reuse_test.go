package list_projection_reuse_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
	webserver "seed-vigor-workbench/internal/web"
)

type sequenceRepository struct {
	mu    sync.Mutex
	calls int
}

func (r *sequenceRepository) List(context.Context) ([]domain.GerminationAssay, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	sample := "ACC-FIRST"
	if r.calls == 2 {
		sample = "ACC-SECOND"
	}
	return []domain.GerminationAssay{{
		ID: "assay-1", SampleAccession: sample, LaboratoryBatchNo: "LAB-1",
		State: domain.StateDraft, Revision: int64(r.calls),
	}}, nil
}

func (*sequenceRepository) Create(context.Context, *domain.GerminationAssay, string) error {
	return errors.New("unexpected Create")
}

func (*sequenceRepository) Get(context.Context, string) (*domain.GerminationAssay, error) {
	return nil, errors.New("unexpected Get")
}

func (*sequenceRepository) Update(context.Context, string, int64, string, string, map[string]any, func(*domain.GerminationAssay) error) (*domain.GerminationAssay, error) {
	return nil, errors.New("unexpected Update")
}

type headerBarrierWriter struct {
	header  http.Header
	body    bytes.Buffer
	status  int
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newHeaderBarrierWriter() *headerBarrierWriter {
	return &headerBarrierWriter{header: make(http.Header), entered: make(chan struct{}), release: make(chan struct{})}
}

func (w *headerBarrierWriter) Header() http.Header { return w.header }

func (w *headerBarrierWriter) WriteHeader(status int) {
	w.status = status
	w.once.Do(func() { close(w.entered) })
	<-w.release
}

func (w *headerBarrierWriter) Write(payload []byte) (int, error) {
	return w.body.Write(payload)
}

func TestConcurrentListResponsesOwnTheirProjection(t *testing.T) {
	repository := &sequenceRepository{}
	service := application.NewService(repository, func(string) string { return "unused" })
	handler := webserver.NewServer(service, "").Handler()
	firstWriter := newHeaderBarrierWriter()
	firstDone := make(chan struct{})

	go func() {
		handler.ServeHTTP(firstWriter, httptest.NewRequest(http.MethodGet, "/api/assays", nil))
		close(firstDone)
	}()

	<-firstWriter.entered
	secondWriter := httptest.NewRecorder()
	handler.ServeHTTP(secondWriter, httptest.NewRequest(http.MethodGet, "/api/assays", nil))
	close(firstWriter.release)
	<-firstDone

	if firstWriter.status != http.StatusOK {
		t.Fatalf("首个列表请求状态码 = %d", firstWriter.status)
	}
	if !bytes.Contains(firstWriter.body.Bytes(), []byte("ACC-FIRST")) {
		t.Fatalf("首个列表响应在编码前被后续请求覆盖: %s", firstWriter.body.String())
	}
}
