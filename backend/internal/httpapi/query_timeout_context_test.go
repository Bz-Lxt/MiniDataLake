package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minidatalake/internal/app"
	"minidatalake/internal/apperr"
	"minidatalake/internal/config"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestQueryTimeoutReturnsPublicTimeoutError(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.MemoryBudgetBytes = 1 << 20
	cfg.BatchSize = 1
	cfg.QueryTimeoutSec = 0

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng, err := app.New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	table := &storage.Table{
		Name: "events", Rows: 1, Status: "READY",
		Cols: []*storage.Column{{
			Meta: storage.ColumnMeta{Name: "id", Type: types.Int64, Rows: 1},
			Vec:  storage.NewInt64([]int64{1}, storage.NewBitmap(1)),
		}},
	}
	if err := eng.Cat.Put(table); err != nil {
		t.Fatal(err)
	}

	s := &Server{Eng: eng, Log: log}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query", strings.NewReader(`{"sql":"SELECT id FROM events"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var body struct {
		Code    apperr.Code `json:"code"`
		Message string      `json:"message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v; body=%q", err, rec.Body.String())
	}
	if rec.Code != http.StatusRequestTimeout || body.Code != apperr.QueryTimeout {
		t.Fatalf("query deadline returned status=%d code=%q message=%q; want status=%d code=%q",
			rec.Code, body.Code, body.Message, http.StatusRequestTimeout, apperr.QueryTimeout)
	}
}
