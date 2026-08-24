package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestStarKeepsTrailingProjection(t *testing.T) {
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
	table := &storage.Table{
		Name: "metrics", Rows: 2, Status: "READY",
		Cols: []*storage.Column{
			{
				Meta: storage.ColumnMeta{Name: "id", Type: types.Int64, Encoding: types.Plain, Rows: 2},
				Vec:  storage.NewInt64([]int64{1, 2}, storage.NewBitmap(2)),
			},
			{
				Meta: storage.ColumnMeta{Name: "value", Type: types.Int64, Encoding: types.Plain, Rows: 2},
				Vec:  storage.NewInt64([]int64{7, 9}, storage.NewBitmap(2)),
			},
		},
	}
	if err := eng.Cat.Put(table); err != nil {
		t.Fatal(err)
	}

	handler := (&httpapi.Server{Eng: eng, Log: logger}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query", strings.NewReader(
		`{"sql":"SELECT *, id + 10 AS shifted FROM metrics"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("query status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Schema []struct {
			Name string `json:"name"`
		} `json:"schema"`
		Rows [][]any `json:"rows"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, len(response.Schema))
	for i := range response.Schema {
		gotNames[i] = response.Schema[i].Name
	}
	if want := []string{"id", "value", "shifted"}; !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("schema names = %v, want %v", gotNames, want)
	}
	wantRows := [][]any{{float64(1), float64(7), float64(11)}, {float64(2), float64(9), float64(12)}}
	if !reflect.DeepEqual(response.Rows, wantRows) {
		t.Fatalf("rows = %v, want %v", response.Rows, wantRows)
	}
}
