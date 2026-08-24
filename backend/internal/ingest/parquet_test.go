package ingest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseParquetFile(t *testing.T) {
	root, _ := os.Getwd()
	// walk up to module
	dir := root
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	out := filepath.Join(t.TempDir(), "s.parquet")
	cmd := exec.Command("go", "run", "./cmd/genparquet", out)
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("genparquet: %v %s", err, b)
	}
	names, _, vecs, err := ParseParquet(context.Background(), out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 || vecs[0].Len() != 4 {
		t.Fatalf("%v len=%d", names, vecs[0].Len())
	}
}
