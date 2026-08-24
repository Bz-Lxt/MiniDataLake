package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
	"minidatalake/internal/persist"
)

func TestRetryJobOutlivesAcceptedRequest(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })

	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.MemoryBudgetBytes = 64 << 20
	cfg.MaxUploadBytes = 16 << 20
	cfg.ChunkBytes = 4 << 10

	eng, err := app.New(cfg, logx.New("error"))
	if err != nil {
		t.Fatal(err)
	}
	handler := (&httpapi.Server{Eng: eng, Log: logx.New("error")}).Handler()

	blocker := filepath.Join(dir, "events.mdl.tmp")
	if err := os.Mkdir(blocker, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := "id,name\n" + strings.Repeat("1,alpha\n", 1<<15)
	job, err := eng.StartIngest("events.csv", "csv", "text/csv", strings.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForTerminalJob(t, handler, job.ID)
	if failed.Status != "FAILED" {
		t.Fatalf("initial job status = %s, want FAILED (error: %s)", failed.Status, failed.Error)
	}
	waitForBudget(t, eng, 0)
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+job.ID+"/retry", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var accepted persist.JobRec
	if err := json.NewDecoder(rec.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Status != "RUNNING" {
		t.Fatalf("accepted retry status = %s, want RUNNING", accepted.Status)
	}

	finished := waitForTerminalJob(t, handler, job.ID)
	if finished.Status != "DONE" {
		t.Fatalf("retried job status = %s, want DONE (phase: %s, error: %s)", finished.Status, finished.Phase, finished.Error)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/tables/"+finished.Table, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("retried table status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func waitForTerminalJob(t *testing.T, handler http.Handler, id string) persist.JobRec {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+id, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("job status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var job persist.JobRec
		if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
			t.Fatal(err)
		}
		if job.Status != "RUNNING" {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s remained RUNNING", id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForBudget(t *testing.T, eng *app.Engine, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for eng.Bud.Used() != want {
		if time.Now().After(deadline) {
			t.Fatalf("budget used = %d, want %d", eng.Bud.Used(), want)
		}
		time.Sleep(time.Millisecond)
	}
}
