package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/logx"
)

type cleanupJobResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Phase  string `json:"phase"`
	Table  string `json:"table"`
	Error  string `json:"error"`
	Reused bool   `json:"reused"`
}

type cleanupStatsResponse struct {
	BudgetUsed int64 `json:"budget_used"`
}

type cleanupTableResponse struct {
	MemBytes int64 `json:"mem_bytes"`
}

func TestReusedUploadReleasesIngestBudget(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:           dir,
		StaticDir:         dir,
		MemoryBudgetBytes: 32 << 20,
		MaxUploadBytes:    1 << 20,
		ResultTTLSeconds:  60,
		PageDefault:       100,
		PageMax:           1000,
		BatchSize:         64,
		ChunkBytes:        4096,
		QueryTimeoutSec:   5,
		DictCardRatio:     0.05,
		RLEMinRun:         8,
		MaxNestedJSON:     6,
	}
	logger := logx.New("error")
	eng, err := app.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Eng: eng, Log: logger}).Handler()

	data := []byte("id,name\n1,alpha\n2,beta\n")
	first, code, body := cleanupUploadCSV(t, h, "seed.csv", data)
	if code != http.StatusOK {
		t.Fatalf("first upload: status=%d body=%s", code, body)
	}
	first = cleanupWaitJob(t, h, first.ID)
	if first.Reused || first.Phase != "ready" {
		t.Fatalf("first job = %+v", first)
	}
	resident := cleanupTableMem(t, h, first.Table)
	baseline, ok := cleanupWaitBudget(t, h, resident, 2*time.Second)
	if !ok {
		t.Fatalf("first ingest reservation did not clear: table_mem=%d budget_used=%d", resident, baseline)
	}

	duplicate, code, body := cleanupUploadCSV(t, h, "seed-copy.csv", data)
	if code != http.StatusOK {
		t.Fatalf("duplicate upload: status=%d body=%s", code, body)
	}
	duplicate = cleanupWaitJob(t, h, duplicate.ID)
	if !duplicate.Reused || duplicate.Phase != "reused" || duplicate.Table != first.Table {
		t.Fatalf("duplicate job = %+v", duplicate)
	}
	after, ok := cleanupWaitBudget(t, h, baseline, 500*time.Millisecond)
	if !ok {
		t.Fatalf("reused upload retained ingest budget: before=%d after=%d", baseline, after)
	}
}

func cleanupUploadCSV(t *testing.T, h http.Handler, name string, data []byte) (cleanupJobResponse, int, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	raw := append([]byte(nil), rec.Body.Bytes()...)
	var job cleanupJobResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(raw, &job); err != nil {
			t.Fatalf("decode upload response: %v; body=%s", err, raw)
		}
	}
	return job, rec.Code, string(raw)
}

func cleanupWaitJob(t *testing.T, h http.Handler, id string) cleanupJobResponse {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+id, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("get job %s: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
		var job cleanupJobResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		switch job.Status {
		case "DONE":
			return job
		case "FAILED", "INTERRUPTED":
			t.Fatalf("job %s ended %s: %s", id, job.Status, job.Error)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
	return cleanupJobResponse{}
}

func cleanupTableMem(t *testing.T, h http.Handler, table string) int64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tables/"+table, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get table %s: status=%d body=%s", table, rec.Code, rec.Body.String())
	}
	var detail cleanupTableResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	return detail.MemBytes
}

func cleanupWaitBudget(t *testing.T, h http.Handler, want int64, timeout time.Duration) (int64, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last int64
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/system/stats", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("stats: status=%d body=%s", rec.Code, rec.Body.String())
		}
		var stats cleanupStatsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
			t.Fatal(err)
		}
		last = stats.BudgetUsed
		if last == want {
			return last, true
		}
		if time.Now().After(deadline) {
			return last, false
		}
		time.Sleep(time.Millisecond)
	}
}
