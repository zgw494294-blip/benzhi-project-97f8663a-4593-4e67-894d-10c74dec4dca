package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
)

type selfcheckView struct {
	Assay struct {
		ID       string `json:"id"`
		State    string `json:"state"`
		Revision int64  `json:"revision"`
		Report   any    `json:"report"`
	} `json:"assay"`
	ReportConsistent bool `json:"report_consistent"`
}

func RunSelfcheck(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 4 * time.Second}
	if err := waitHealthy(ctx, client, baseURL); err != nil {
		return err
	}
	create := application.CreateAssayCommand{
		SampleAccession: "SELFCHECK-SEED", LaboratoryBatchNo: "SELFCHECK-BATCH",
		OperatorName: "自检检验员", ReviewerName: "自检复核员",
	}
	create.Protocol.TemperatureCelsius = 25
	create.Protocol.Substrate = "湿润滤纸"
	create.Protocol.LightCycleHours = 12
	create.Protocol.ObservationDays = 2
	create.Protocol.ReplicateCount = 2
	create.Protocol.SeedsPerReplicate = 10
	create.Protocol.DispersionLimit = 0.2
	create.Protocol.NormalSeedlingRule = "根系与胚轴完整"
	view := selfcheckView{}
	if err := postJSON(ctx, client, baseURL+"/api/assays", create, http.StatusCreated, &view); err != nil {
		return fmt.Errorf("自检建档失败: %w", err)
	}
	id := view.Assay.ID
	freeze := application.RevisionCommand{ExpectedRevision: view.Assay.Revision, Principal: application.Principal{Name: "自检检验员", Role: application.RoleOperator}}
	if err := postJSON(ctx, client, baseURL+"/api/assays/"+id+"/freeze", freeze, http.StatusOK, &view); err != nil {
		return fmt.Errorf("自检冻结失败: %w", err)
	}
	for day := 1; day <= 2; day++ {
		normal := 5 + day*2
		observation := application.DailyObservationCommand{ExpectedRevision: view.Assay.Revision, DayNo: day, RecordedBy: "自检检验员",
			Observations: []application.ObservationReading{{ReplicateNo: 1, NormalCount: normal, UngerminatedCount: 10 - normal}, {ReplicateNo: 2, NormalCount: normal, UngerminatedCount: 10 - normal}}}
		if err := postJSON(ctx, client, baseURL+"/api/assays/"+id+"/observations/day", observation, http.StatusOK, &view); err != nil {
			return fmt.Errorf("自检观察日 %d 失败: %w", day, err)
		}
	}
	seal := application.RevisionCommand{ExpectedRevision: view.Assay.Revision, Principal: application.Principal{Name: "自检检验员", Role: application.RoleOperator}}
	if err := postJSON(ctx, client, baseURL+"/api/assays/"+id+"/seal", seal, http.StatusOK, &view); err != nil {
		return fmt.Errorf("自检封存失败: %w", err)
	}
	checklist := domain.DefaultReviewChecklist()
	for index := range checklist {
		checklist[index].Status = "passed"
	}
	approve := application.ReviewCommand{ExpectedRevision: view.Assay.Revision, Reviewer: "自检复核员", Opinion: "读数和推导过程符合规则，批准归档", Checklist: checklist}
	if err := postJSON(ctx, client, baseURL+"/api/assays/"+id+"/review/approve", approve, http.StatusOK, &view); err != nil {
		return fmt.Errorf("自检归档失败: %w", err)
	}
	if view.Assay.State != "archived" || view.Assay.Report == nil || !view.ReportConsistent {
		return fmt.Errorf("自检归档结果不完整：state=%s consistent=%v", view.Assay.State, view.ReportConsistent)
	}
	return nil
}

func waitHealthy(ctx context.Context, client *http.Client, baseURL string) error {
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		response, err := client.Do(request)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 HTTP 服务就绪: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func postJSON(ctx context.Context, client *http.Client, url string, input any, expected int, output any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != expected {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, output); err != nil {
		return err
	}
	return nil
}
