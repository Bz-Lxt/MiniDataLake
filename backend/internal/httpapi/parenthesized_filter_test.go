package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestParenthesizedWherePreservesFilter(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.PageDefault = 100
	cfg.PageMax = 100
	cfg.QueryTimeoutSec = 5

	logger := logx.New("error")
	eng, err := app.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	ids := storage.NewInt64([]int64{1, 2, 3}, nil)
	amounts := storage.NewInt64([]int64{100, 1500, 2400}, nil)
	table := &storage.Table{
		Name: "payments", Rows: 3, Status: "READY",
		Cols: []*storage.Column{
			{
				Meta: storage.ColumnMeta{
					Name: "id", Type: types.Int64, Encoding: types.Plain, Rows: 3,
					RawBytes: ids.RawBytes(), EncBytes: ids.MemBytes(),
				},
				Vec: ids,
			},
			{
				Meta: storage.ColumnMeta{
					Name: "amount", Type: types.Int64, Encoding: types.Plain, Rows: 3,
					RawBytes: amounts.RawBytes(), EncBytes: amounts.MemBytes(),
				},
				Vec: amounts,
			},
		},
	}
	if err := eng.Cat.Put(table); err != nil {
		t.Fatal(err)
	}

	server := (&httpapi.Server{Eng: eng, Log: logger}).Handler()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/query",
		strings.NewReader(`{"sql":"SELECT id FROM payments WHERE (amount > 1000) ORDER BY id"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("query returned %d: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Rows      [][]int64 `json:"rows"`
		TotalRows int       `json:"total_rows"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.TotalRows != 2 || len(got.Rows) != 2 ||
		len(got.Rows[0]) != 1 || got.Rows[0][0] != 2 ||
		len(got.Rows[1]) != 1 || got.Rows[1][0] != 3 {
		t.Fatalf("parenthesized filter returned rows=%v total_rows=%d", got.Rows, got.TotalRows)
	}
}
