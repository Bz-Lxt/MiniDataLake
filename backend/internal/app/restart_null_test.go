package app_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/resultset"
)

func TestDictionaryNullSurvivesEngineRestart(t *testing.T) {
	cfg := config.Load()
	cfg.DataDir = t.TempDir()
	cfg.StaticDir = cfg.DataDir
	cfg.MemoryBudgetBytes = 64 << 20
	cfg.MaxUploadBytes = 1 << 20
	cfg.ChunkBytes = 1 << 20
	cfg.BatchSize = 64
	cfg.QueryTimeoutSec = 5
	cfg.DictCardRatio = 1
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	eng, err := app.New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	raw := "id,region\n1,north\n2,\n3,south\n4,north\n5,south\n6,north\n"
	job, err := eng.StartIngest("customers.csv", "csv", "text/csv", strings.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	table := waitForCompletedImport(t, eng, job.ID)
	assertNullRegion(t, eng, table)

	reloaded, err := app.New(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	assertNullRegion(t, reloaded, table)
}

func waitForCompletedImport(t *testing.T, eng *app.Engine, id string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := eng.Store.Job(id)
		if ok && job.Status == "DONE" {
			return job.Table
		}
		if ok && job.Status != "RUNNING" {
			t.Fatalf("import ended in %s: %s", job.Status, job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("import %s did not complete", id)
	return ""
}

func assertNullRegion(t *testing.T, eng *app.Engine, table string) {
	t.Helper()
	it, err := eng.Query(context.Background(), "SELECT region FROM "+table+" WHERE id = 2")
	if err != nil {
		t.Fatal(err)
	}
	rows := resultset.Page(it, 0, 10)
	if len(rows) != 1 || len(rows[0]) != 1 {
		t.Fatalf("unexpected query result: %#v", rows)
	}
	if rows[0][0] != nil {
		t.Fatalf("region changed after reload: got %#v, want null", rows[0][0])
	}
}
