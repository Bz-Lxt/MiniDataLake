package httpapi_test

import (
	"encoding/json"
	"io"
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

func TestQueryResultRemainsAvailableAfterInitialResponse(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	cfg.APIToken = ""
	cfg.ResultTTLSeconds = 600
	cfg.PageDefault = 1
	cfg.PageMax = 100
	cfg.BatchSize = 16
	cfg.QueryTimeoutSec = 5
	eng, err := app.New(cfg, logx.New("error"))
	if err != nil {
		t.Fatal(err)
	}

	ids := storage.NewInt64([]int64{101, 102}, nil)
	table := &storage.Table{
		Name: "events", Rows: 2, Status: "READY",
		Cols: []*storage.Column{{
			Meta: storage.ColumnMeta{
				Name: "id", Type: types.Int64, Encoding: types.Plain, Rows: 2,
				RawBytes: ids.RawBytes(), EncBytes: ids.MemBytes(),
			},
			Vec: ids,
		}},
	}
	if err := eng.Cat.Put(table); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer((&httpapi.Server{Eng: eng, Log: logx.New("error")}).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Post(
		srv.URL+"/api/v1/query",
		"application/json",
		strings.NewReader(`{"sql":"SELECT id FROM events"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query status %d: %s", resp.StatusCode, body)
	}
	var first struct {
		ResultSetID string  `json:"result_set_id"`
		Rows        [][]any `json:"rows"`
	}
	if err := json.Unmarshal(body, &first); err != nil {
		t.Fatal(err)
	}
	if first.ResultSetID == "" || len(first.Rows) != 1 {
		t.Fatalf("unexpected query response: %s", body)
	}

	resp, err = http.Get(srv.URL + "/api/v1/results/" + first.ResultSetID + "?offset=1&limit=1")
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("result page status %d: %s", resp.StatusCode, body)
	}
	var page struct {
		TotalRows int     `json:"total_rows"`
		Rows      [][]any `json:"rows"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if page.TotalRows != 2 || len(page.Rows) != 1 {
		t.Fatalf("unexpected result page: %s", body)
	}
}
