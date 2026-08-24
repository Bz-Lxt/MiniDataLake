package httpapi_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/apperr"
	"minidatalake/internal/catalog"
	"minidatalake/internal/config"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
	"minidatalake/internal/memgov"
	"minidatalake/internal/persist"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

type blockingVector struct {
	storage.Vector
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (v *blockingVector) MemBytes() int64 {
	v.once.Do(func() { close(v.entered) })
	<-v.release
	return v.Vector.MemBytes()
}

type httpResult struct {
	request string
	status  int
	body    []byte
}

func TestConcurrentDeleteAndRejectedUploadDoNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.MemoryBudgetBytes = 1024
	cfg.MaxUploadBytes = 1 << 20

	store, err := persist.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var cat *catalog.Catalog
	hintEntered := make(chan struct{})
	var hintOnce sync.Once
	budget := memgov.New(cfg.MemoryBudgetBytes, func() []memgov.Victim {
		hintOnce.Do(func() { close(hintEntered) })
		return cat.Victims()
	})
	cat, err = catalog.Open(dir, store, budget)
	if err != nil {
		t.Fatal(err)
	}

	vec := storage.NewInt64([]int64{1}, storage.NewBitmap(1))
	table := &storage.Table{
		Name: "wide", Rows: 1, Status: "READY",
		Cols: []*storage.Column{{
			Meta: storage.ColumnMeta{
				Name: "id", Type: types.Int64, Encoding: types.Plain, Rows: 1,
				RawBytes: vec.RawBytes(), EncBytes: vec.MemBytes(),
			},
			Vec: vec,
		}},
	}
	if err := cat.Put(table); err != nil {
		t.Fatal(err)
	}
	if err := budget.Reserve(table.MemBytes()); err != nil {
		t.Fatal(err)
	}

	memBytesEntered := make(chan struct{})
	releaseMemBytes := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseMemBytes) }) })
	table.Cols[0].Vec = &blockingVector{
		Vector:  vec,
		entered: memBytesEntered,
		release: releaseMemBytes,
	}

	logger := logx.New("error")
	eng := &app.Engine{Cfg: cfg, Log: logger, Cat: cat, Store: store, Bud: budget}
	handler := (&httpapi.Server{Eng: eng, Log: logger}).Handler()
	done := make(chan httpResult, 2)

	go func() {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/tables/wide", nil)
		handler.ServeHTTP(w, r)
		done <- httpResult{request: "delete", status: w.Code, body: append([]byte(nil), w.Body.Bytes()...)}
	}()

	select {
	case <-memBytesEntered:
	case <-time.After(time.Second):
		t.Fatal("delete did not reach table memory accounting")
	}

	upload := newCSVUploadRequest(t)
	go func() {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, upload)
		done <- httpResult{request: "upload", status: w.Code, body: append([]byte(nil), w.Body.Bytes()...)}
	}()

	select {
	case <-hintEntered:
	case <-time.After(time.Second):
		t.Fatal("upload did not reach the memory rejection path")
	}
	releaseOnce.Do(func() { close(releaseMemBytes) })

	responses := make(map[string]httpResult, 2)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for len(responses) < 2 {
		select {
		case res := <-done:
			responses[res.request] = res
		case <-timer.C:
			t.Fatal("delete and rejected upload did not both complete")
		}
	}

	if res := responses["delete"]; res.status != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", res.status, res.body)
	}
	uploadRes := responses["upload"]
	if uploadRes.status != http.StatusInsufficientStorage {
		t.Fatalf("upload status = %d, body = %s", uploadRes.status, uploadRes.body)
	}
	var apiErr struct {
		Code apperr.Code `json:"code"`
	}
	if err := json.Unmarshal(uploadRes.body, &apiErr); err != nil {
		t.Fatal(err)
	}
	if apiErr.Code != apperr.InsufficientMemory {
		t.Fatalf("upload error code = %q, body = %s", apiErr.Code, uploadRes.body)
	}
}

func newCSVUploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "over-budget.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("id\n1\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}
