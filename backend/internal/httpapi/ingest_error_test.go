package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/logx"
)

func TestInvalidCSVReportsParseError(t *testing.T) {
	dir := t.TempDir()
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
	handler := (&Server{Eng: eng, Log: logx.New("error")}).Handler()

	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	if _, err := form.CreateFormFile("file", "empty.csv"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &upload)
	req.Header.Set("Content-Type", form.FormDataContentType())
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("upload response has no job id: %s", res.Body.String())
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID, nil)
		res = httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("job status=%d body=%s", res.Code, res.Body.String())
		}
		var job struct {
			Status string `json:"status"`
			Phase  string `json:"phase"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.Status == "RUNNING" {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if job.Status != "FAILED" {
			t.Fatalf("status=%q phase=%q error=%q", job.Status, job.Phase, job.Error)
		}
		if job.Phase == "panic" {
			t.Fatalf("invalid CSV ended in panic phase: error=%q", job.Error)
		}
		if !strings.Contains(job.Error, "csv has no header") {
			t.Fatalf("job lost parse error: phase=%q error=%q", job.Phase, job.Error)
		}
		return
	}
	t.Fatal("ingest job did not reach a terminal state")
}
