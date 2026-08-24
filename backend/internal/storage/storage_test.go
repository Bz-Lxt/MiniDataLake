package storage

import (
	"testing"

	"minidatalake/internal/types"
)

func TestBitmapAndTake(t *testing.T) {
	b := NewBitmap(0)
	b.Append(false)
	b.Append(true)
	b.Append(false)
	if !b.Get(1) || b.Get(0) || b.Count() != 1 {
		t.Fatalf("bitmap %+v", b)
	}
	v := NewInt64([]int64{1, 2, 3}, b)
	got := v.Take([]int{1, 2})
	if got.Len() != 2 || !got.IsNull(0) || got.Get(1).I != 3 {
		t.Fatalf("take %+v %+v", got.Get(0), got.Get(1))
	}
}

func TestStrArena(t *testing.T) {
	sv := BuildStr([]string{"a", "bb", "a"}, []bool{false, false, true})
	if sv.At(1) != "bb" || !sv.IsNull(2) {
		t.Fatal(sv.Get(1), sv.Get(2))
	}
	if sv.Encoding() != types.Plain {
		t.Fatal(sv.Encoding())
	}
}

func TestBuilderConcat(t *testing.T) {
	a := FromValues(types.Int64, []types.Value{types.VInt(1), types.Null(types.Int64)})
	b := FromValues(types.Int64, []types.Value{types.VInt(9)})
	c := Concat([]Vector{a, b})
	if c.Len() != 3 || c.Get(2).I != 9 || !c.IsNull(1) {
		t.Fatal(c.Len(), c.Get(2), c.IsNull(1))
	}
}
