package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
)

func TestIngestDoesNotSucceedWhenTableCommitFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "orders.mdl"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.MemoryBudgetBytes = 64 << 20
	cfg.MaxUploadBytes = 1 << 20
	eng, err := app.New(cfg, logx.New("error"))
	if err != nil {
		t.Fatal(err)
	}
	handler := (&httpapi.Server{Eng: eng, Log: logx.New("error")}).Handler()

	jobID := uploadCSV(t, handler, "orders.csv", "id,name\n1,Ada\n")
	status := waitForJob(t, handler, jobID)
	if status != "FAILED" {
		t.Errorf("commit failure reported job status %q, want FAILED", status)
	}

	restarted, err := app.New(cfg, logx.New("error"))
	if err != nil {
		t.Fatal(err)
	}
	restartedHandler := (&httpapi.Server{Eng: restarted, Log: logx.New("error")}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tables/orders", nil)
	rec := httptest.NewRecorder()
	restartedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("table was published after its commit failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func uploadCSV(t *testing.T, handler http.Handler, name, content string) string {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
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
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	var job struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	if job.ID == "" {
		t.Fatal("upload response did not include a job id")
	}
	return job.ID
}

func waitForJob(t *testing.T, handler http.Handler, id string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+id, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("job status=%d body=%s", rec.Code, rec.Body.String())
		}
		var job struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
			t.Fatal(err)
		}
		if job.Status != "RUNNING" {
			return job.Status
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s remained RUNNING", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
