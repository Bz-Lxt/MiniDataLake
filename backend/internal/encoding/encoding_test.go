package encoding

import (
	"testing"

	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestDictRoundtrip(t *testing.T) {
	ss := make([]string, 100)
	for i := range ss {
		ss[i] = []string{"beijing", "shanghai", "shenzhen"}[i%3]
	}
	src := storage.BuildStr(ss, nil)
	d := EncodeDict(src)
	if d.Cardinality() != 3 {
		t.Fatal(d.Cardinality())
	}
	if d.Get(5).S != "shenzhen" {
		t.Fatal(d.Get(5))
	}
	if d.MemBytes() >= src.MemBytes() {
		t.Fatalf("dict should be smaller %d vs %d", d.MemBytes(), src.MemBytes())
	}
	id, ok := d.LookupID("shanghai")
	if !ok || d.Get(1).S != "shanghai" {
		t.Fatal(id, ok)
	}
}

func TestRLESeek(t *testing.T) {
	vals := make([]int64, 80)
	for i := range vals {
		vals[i] = int64(i / 10)
	}
	src := storage.NewInt64(vals, storage.NewBitmap(80))
	if AvgRun(src) < 8 {
		t.Fatal(AvgRun(src))
	}
	r := EncodeRLE(src)
	if r.Get(25).I != 2 || r.Get(79).I != 7 {
		t.Fatal(r.Get(25), r.Get(79))
	}
	if r.MemBytes() >= src.MemBytes() {
		t.Fatalf("rle %d vs plain %d", r.MemBytes(), src.MemBytes())
	}
}

func TestChoose(t *testing.T) {
	ss := make([]string, 200)
	for i := range ss {
		ss[i] = "city"
	}
	ch := Choose("c", storage.BuildStr(ss, nil), 0.05, 8, nil)
	if ch.Meta.Encoding != types.Dict && ch.Meta.Encoding != types.RLE {
		t.Fatal(ch.Meta.Encoding, ch.Meta.Reason)
	}
}
