package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"minidatalake/internal/config"
	"minidatalake/internal/types"
)

func TestParseCSVEndToEnd(t *testing.T) {
	raw := "name,age,city\nAda,30,bj\nBob,xx,sh\nCara,22,bj\n"
	r := ra{[]byte(raw)}
	ch, err := SplitCSV(r, int64(len(raw)), 16, ',')
	if err != nil {
		t.Fatal(err)
	}
	tys := []types.DataType{types.String, types.Int64, types.String}
	vecs, _, err := ParseCSV(context.Background(), ch, []string{"name", "age", "city"}, tys, ',', true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if vecs[0].Len() != 3 {
		t.Fatal(vecs[0].Len())
	}
}

func TestJSONFlatten(t *testing.T) {
	raw := `{"user":{"id":1},"city":"bj"}
{"user":{"id":2},"city":"sh"}
`
	names, _, vecs, _, err := ParseJSON(context.Background(), strings.NewReader(raw), 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 2 || vecs[0].Len() != 2 {
		t.Fatal(names, vecs[0].Len())
	}
}

func TestIngestCSV(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.csv")
	if err := os.WriteFile(p, []byte("age,city\n21,bj\n21,bj\n33,sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tbl, err := Ingest(context.Background(), p, Options{Filename: "s.csv", Format: "csv", Cfg: config.Load()})
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Rows != 3 {
		t.Fatal(tbl.Rows)
	}
}
