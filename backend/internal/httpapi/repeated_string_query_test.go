package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/logx"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestRepeatedStringQueryKeepsTableData(t *testing.T) {
	cfg := config.Load()
	cfg.DataDir = t.TempDir()
	cfg.StaticDir = cfg.DataDir
	cfg.APIToken = ""
	cfg.BatchSize = 2
	cfg.PageDefault = 100
	cfg.PageMax = 100
	cfg.QueryTimeoutSec = 5
	log := logx.New("error")
	eng, err := app.New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "bb", "ccc", "dddd", "eeeee", "ffffff"}
	table := &storage.Table{
		Name:   "labels",
		Rows:   len(want),
		Status: "READY",
		Cols: []*storage.Column{{
			Meta: storage.ColumnMeta{Name: "label", Type: types.String, Rows: len(want)},
			Vec:  storage.BuildStr(want, nil),
		}},
	}
	if err := eng.Cat.Put(table); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer((&Server{Eng: eng, Log: log}).Handler())
	defer srv.Close()
	if got := queryStringColumn(t, srv.Client(), srv.URL, "SELECT label FROM labels", "label"); !equalStrings(got, want) {
		t.Fatalf("first query = %q, want %q", got, want)
	}
	if got := queryStringColumn(t, srv.Client(), srv.URL, "SELECT label FROM labels LIMIT 2", "label"); !equalStrings(got, want[:2]) {
		t.Fatalf("second query = %q, want %q", got, want[:2])
	}
}

func queryStringColumn(t *testing.T, client *http.Client, baseURL, sql, column string) []string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"sql": sql})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(
		baseURL+"/api/v1/query",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("query status %d: %s", resp.StatusCode, body)
	}
	var result struct {
		Schema []struct {
			Name string `json:"name"`
		} `json:"schema"`
		Rows [][]json.RawMessage `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	col := -1
	for i := range result.Schema {
		if result.Schema[i].Name == column {
			col = i
			break
		}
	}
	if col < 0 {
		t.Fatalf("column %q missing from schema: %+v", column, result.Schema)
	}
	values := make([]string, len(result.Rows))
	for i, row := range result.Rows {
		if col >= len(row) {
			t.Fatalf("short result row: %s", row)
		}
		if err := json.Unmarshal(row[col], &values[i]); err != nil {
			t.Fatalf("decode row %d: %v", i, err)
		}
	}
	return values
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
