package httpapi_test

import (
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestQueryWithNoMatchingRowsReturnsEmptyResult(t *testing.T) {
	cfg := config.Load()
	cfg.DataDir = t.TempDir()
	cfg.StaticDir = cfg.DataDir
	cfg.APIToken = ""
	cfg.QueryTimeoutSec = 5
	cfg.PageDefault = 100
	cfg.PageMax = 100

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng, err := app.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	ids := storage.NewInt64([]int64{1, 2}, nil)
	table := &storage.Table{
		Name: "events", Rows: ids.Len(), Status: "READY",
		Cols: []*storage.Column{{
			Meta: storage.ColumnMeta{Name: "id", Type: types.Int64, Rows: ids.Len()},
			Vec:  ids,
		}},
	}
	if err := eng.Cat.Put(table); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewUnstartedServer((&httpapi.Server{Eng: eng, Log: logger}).Handler())
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Start()
	defer server.Close()

	resp, err := server.Client().Post(
		server.URL+"/api/v1/query",
		"application/json",
		strings.NewReader(`{"sql":"SELECT id FROM events WHERE id = 999"}`),
	)
	if err != nil {
		t.Fatalf("empty-result query failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var got struct {
		Schema    []map[string]string `json:"schema"`
		TotalRows int                 `json:"total_rows"`
		Rows      [][]any             `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Schema) != 1 || got.Schema[0]["name"] != "id" {
		t.Fatalf("schema = %#v", got.Schema)
	}
	if got.TotalRows != 0 || len(got.Rows) != 0 {
		t.Fatalf("total_rows = %d, rows = %#v", got.TotalRows, got.Rows)
	}
}
