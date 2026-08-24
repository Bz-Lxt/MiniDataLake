package storage

import (
	"testing"

	"minidatalake/internal/types"
)

func TestPlainTakesAndBatch(t *testing.T) {
	f := NewFloat64([]float64{1.5, 2.5}, NewBitmap(2))
	if f.Take([]int{1}).Get(0).F != 2.5 {
		t.Fatal()
	}
	b := NewBool([]uint8{1, 0}, NewBitmap(2))
	if !b.Get(0).B || b.Take([]int{1}).Get(0).B {
		t.Fatal()
	}
	tm := NewTime([]int64{1, 2}, NewBitmap(2))
	if tm.Get(1).I != 2 {
		t.Fatal()
	}
	sv := BuildStr([]string{"x", "y"}, nil)
	if sv.Take([]int{1}).Get(0).S != "y" {
		t.Fatal()
	}
	nb := NewBitmap(2)
	nb.Set(0)
	if BitmapFromBytes(nb.Bytes(), 2).Get(0) != true {
		t.Fatal()
	}
	if !nb.CloneRange([]int{0}).Get(0) {
		t.Fatal()
	}
	tbl := &Table{Name: "t", Rows: 2, Cols: []*Column{{Meta: ColumnMeta{Name: "f", Type: types.Float64, RawBytes: 16}, Vec: f}}}
	if tbl.RawBytes() != 16 {
		t.Fatal(tbl.RawBytes())
	}
	if _, _, ok := tbl.ColByName("no"); ok {
		t.Fatal()
	}
	bat := NewBatch([]string{"f"}, []Vector{f})
	if _, ok := bat.Col("f"); !ok || bat.Rows != 2 {
		t.Fatal()
	}
	meta := ColumnMeta{RawBytes: 10, EncBytes: 4}
	if meta.Ratio() != 0.4 {
		t.Fatal(meta.Ratio())
	}
}

func TestBuilderNulls(t *testing.T) {
	b := NewBuilder(types.Bool)
	b.Append(types.VBool(true))
	b.AppendNull()
	v := b.Finish()
	if v.Len() != 2 || !v.IsNull(1) {
		t.Fatal()
	}
}
