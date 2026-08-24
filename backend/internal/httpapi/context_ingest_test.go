package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

type cancelAfterBodyReader struct {
	r      *bytes.Reader
	cancel context.CancelFunc
	done   bool
}

func (r *cancelAfterBodyReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 && !r.done && r.r.Len() == 0 {
		r.done = true
		r.cancel()
	}
	return n, err
}

func TestAcceptedUploadOutlivesRequestContext(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.MemoryBudgetBytes = 64 << 20
	cfg.MaxUploadBytes = 1 << 20
	cfg.ChunkBytes = 4096

	logger := logx.New("error")
	eng, err := app.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	handler := (&httpapi.Server{Eng: eng, Log: logger}).Handler()

	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	part, err := form.CreateFormFile("file", "events.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "id,name\n1,alpha\n2,beta\n"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("format", "csv"); err != nil {
		t.Fatal(err)
	}
	contentType := form.FormDataContentType()
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &cancelAfterBodyReader{r: bytes.NewReader(upload.Bytes()), cancel: cancel}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", body).WithContext(ctx)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := ctx.Err(); err != context.Canceled {
		t.Fatalf("request context was not canceled after the body was read: %v", err)
	}

	var accepted struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode upload response: %v; body=%s", err, rec.Body.String())
	}
	if accepted.ID == "" || accepted.Status != "RUNNING" {
		t.Fatalf("unexpected accepted job: %+v", accepted)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		jobReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+accepted.ID, nil)
		jobRec := httptest.NewRecorder()
		handler.ServeHTTP(jobRec, jobReq)
		if jobRec.Code != http.StatusOK {
			t.Fatalf("job status=%d body=%s", jobRec.Code, jobRec.Body.String())
		}
		var job struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(jobRec.Body.Bytes(), &job); err != nil {
			t.Fatalf("decode job response: %v; body=%s", err, jobRec.Body.String())
		}
		switch job.Status {
		case "DONE":
			return
		case "FAILED", "INTERRUPTED":
			t.Fatalf("accepted upload ended as %s: %s", job.Status, job.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("accepted upload remained %s", job.Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
