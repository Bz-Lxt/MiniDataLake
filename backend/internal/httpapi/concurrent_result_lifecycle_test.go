package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/exec"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
	"minidatalake/internal/resultset"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

type pausedResponseWriter struct {
	header  http.Header
	body    bytes.Buffer
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (w *pausedResponseWriter) Header() http.Header { return w.header }

func (w *pausedResponseWriter) WriteHeader(int) {}

func (w *pausedResponseWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return w.body.Write(p)
}

func TestDeleteResultDoesNotAbortInFlightExport(t *testing.T) {
	cfg := config.Load()
	cfg.DataDir = t.TempDir()
	cfg.StaticDir = cfg.DataDir
	cfg.APIToken = ""
	log := logx.New("error")
	eng, err := app.New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}

	const resultID = "in-flight-result"
	vec := storage.NewInt64([]int64{1, 2}, nil)
	eng.RS.Put(&resultset.Item{
		ID:      resultID,
		Created: time.Now(),
		Res: &exec.Result{
			Names: []string{"id"},
			Types: []types.DataType{types.Int64},
			Cols:  []storage.Vector{vec},
			Rows:  2,
		},
	})

	handler := (&httpapi.Server{Eng: eng, Log: log}).Handler()
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	w := &pausedResponseWriter{
		header:  make(http.Header),
		started: make(chan struct{}),
		release: release,
	}

	panicCh := make(chan any, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { panicCh <- recover() }()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/results/"+resultID+"/export", nil)
		handler.ServeHTTP(w, r)
	}()

	select {
	case <-w.started:
	case <-time.After(2 * time.Second):
		t.Fatal("export did not start writing")
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/results/"+resultID, nil)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", delRec.Code, delRec.Body.String())
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("export did not finish after delete")
	}
	if p := <-panicCh; p != nil {
		t.Fatalf("in-flight export panicked after delete: %v", p)
	}
	if got, want := w.body.String(), "id\n1\n2\n"; got != want {
		t.Fatalf("export body = %q, want %q", got, want)
	}
	if got := w.Header().Get("Content-Disposition"); got != "attachment; filename=result.csv" {
		t.Fatalf("Content-Disposition = %q", got)
	}
}
