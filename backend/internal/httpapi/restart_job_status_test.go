package httpapi_test

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
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
)

type restartStatusJob struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Table  string `json:"table"`
	Error  string `json:"error"`
}

func TestCompletedJobStatusSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.MemoryBudgetBytes = 64 << 20
	cfg.MaxUploadBytes = 1 << 20
	cfg.ResultTTLSeconds = 60
	cfg.PageDefault = 100
	cfg.PageMax = 100
	cfg.BatchSize = 16
	cfg.ChunkBytes = 4096
	cfg.QueryTimeoutSec = 5
	cfg.DictCardRatio = 0.05
	cfg.RLEMinRun = 8

	first := newRestartStatusHandler(t, cfg)
	job := uploadRestartStatusCSV(t, first, "name,score\nalpha,1\nbeta,2\n")
	job = waitForRestartStatusJob(t, first, job.ID)
	if got := restartStatusQueryRows(t, first, job.Table); got != 2 {
		t.Fatalf("before restart: got %d rows, want 2", got)
	}

	second := newRestartStatusHandler(t, cfg)
	if got := restartStatusQueryRows(t, second, job.Table); got != 2 {
		t.Fatalf("after restart: got %d rows, want 2", got)
	}
	restarted := getRestartStatusJob(t, second, job.ID)
	if restarted.Status != "DONE" {
		t.Fatalf("after restart: got job status %s, want DONE", restarted.Status)
	}
}

func newRestartStatusHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	log := logx.New("error")
	eng, err := app.New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	return (&httpapi.Server{Eng: eng, Log: log}).Handler()
}

func uploadRestartStatusCSV(t *testing.T, handler http.Handler, contents string) restartStatusJob {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "completed.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload returned %d: %s", rec.Code, rec.Body.String())
	}
	var job restartStatusJob
	if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	if job.ID == "" {
		t.Fatal("upload response did not include a job id")
	}
	return job
}

func waitForRestartStatusJob(t *testing.T, handler http.Handler, id string) restartStatusJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		job := getRestartStatusJob(t, handler, id)
		switch job.Status {
		case "DONE":
			if job.Table == "" {
				t.Fatal("completed ingest did not include a table name")
			}
			return job
		case "FAILED", "INTERRUPTED":
			t.Fatalf("ingest ended in %s: %s", job.Status, job.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for ingest job %s (last status %s)", id, job.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func getRestartStatusJob(t *testing.T, handler http.Handler, id string) restartStatusJob {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+id, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("job lookup returned %d: %s", rec.Code, rec.Body.String())
	}
	var job restartStatusJob
	if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	return job
}

func restartStatusQueryRows(t *testing.T, handler http.Handler, table string) int {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"sql": "SELECT name, score FROM " + table,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("query returned %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		TotalRows int `json:"total_rows"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response.TotalRows
}
