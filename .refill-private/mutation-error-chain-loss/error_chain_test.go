package mutation_error_chain_loss_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
	"seed-vigor-workbench/internal/persistence"
	webserver "seed-vigor-workbench/internal/web"
)

func TestMutationErrorChainPreservesRevisionConflict(t *testing.T) {
	store, err := persistence.Open(filepath.Join(t.TempDir(), "error-chain.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	sequence := 0
	service := application.NewService(store, func(prefix string) string {
		sequence++
		return fmt.Sprintf("%s-private-%d", prefix, sequence)
	})
	created, err := service.CreateAssay(context.Background(), application.CreateAssayCommand{
		SampleAccession:   "CHAIN-SEED",
		LaboratoryBatchNo: "CHAIN-BATCH",
		OperatorName:      "检验员甲",
		ReviewerName:      "复核员乙",
		Protocol: domain.AssayProtocol{
			TemperatureCelsius: 25,
			Substrate:          "湿润滤纸",
			LightCycleHours:    12,
			ObservationDays:    2,
			ReplicateCount:     2,
			SeedsPerReplicate:  10,
			DispersionLimit:    0.2,
			NormalSeedlingRule: "根系与胚轴完整",
		},
	})
	if err != nil {
		t.Fatalf("create assay: %v", err)
	}

	principal := application.Principal{Name: "检验员甲", Role: application.RoleOperator}
	if _, err := service.FreezeProtocol(context.Background(), created.ID, application.RevisionCommand{
		ExpectedRevision: created.Revision,
		Principal:        principal,
	}); err != nil {
		t.Fatalf("freeze assay: %v", err)
	}

	payload, err := json.Marshal(application.RevisionCommand{
		ExpectedRevision: created.Revision,
		Principal:        principal,
	})
	if err != nil {
		t.Fatalf("marshal stale command: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/assays/"+created.ID+"/freeze", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	webserver.NewServer(service, "").Handler().ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("stale revision must preserve ConflictError through the HTTP boundary: status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code            string `json:"code"`
		CurrentRevision int64  `json:"current_revision"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if body.Code != "revision_conflict" || body.CurrentRevision != created.Revision+1 {
		t.Fatalf("unexpected conflict response: %+v", body)
	}
}
