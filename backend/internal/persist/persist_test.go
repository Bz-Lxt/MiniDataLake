package persist

import (
	"os"
	"path/filepath"
	"testing"

	"minidatalake/internal/encoding"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestWriteReadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.mdl")
	sv := storage.BuildStr([]string{"x", "x", "y"}, nil)
	dv := encoding.EncodeDict(sv)
	tbl := &storage.Table{Name: "t", Rows: 3, Status: "READY", Cols: []*storage.Column{
		{Meta: storage.ColumnMeta{Name: "c", Type: types.String, Encoding: types.Dict, Rows: 3, EncBytes: dv.MemBytes(), RawBytes: sv.MemBytes()}, Vec: dv},
	}}
	if err := WriteTable(path, tbl); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTable(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rows != 3 || got.Cols[0].Vec.Get(2).S != "y" {
		t.Fatal(got.Cols[0].Vec.Get(2))
	}
}

func TestBadVersionAndCRC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.mdl")
	tbl := &storage.Table{Rows: 1, Cols: []*storage.Column{
		{Meta: storage.ColumnMeta{Name: "n", Type: types.Int64}, Vec: storage.NewInt64([]int64{1}, storage.NewBitmap(1))},
	}}
	if err := WriteTable(path, tbl); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	b[4] = 9
	_ = os.WriteFile(path, b, 0644)
	if _, err := ReadTable(path); err == nil {
		t.Fatal("expected version error")
	}
	b[4] = 1
	if len(b) > 40 {
		b[40] ^= 0xff
	}
	_ = os.WriteFile(path, b, 0644)
	if _, err := ReadTable(path); err == nil {
		t.Fatal("expected crc error")
	}
}

func TestInterruptedJobs(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.UpsertJob(JobRec{ID: "j1", Status: "RUNNING"})
	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	j, ok := s2.Job("j1")
	if !ok || j.Status != "INTERRUPTED" {
		t.Fatalf("%+v", j)
	}
}
