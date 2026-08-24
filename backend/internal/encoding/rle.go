package encoding

import (
	"sort"

	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

// RLEVec stores runs of identical values with start-row index for O(log n) seek.
type RLEVec struct {
	typ    types.DataType
	starts []int32
	vals   []types.Value
	n      int
}

func NewRLE(typ types.DataType, starts []int32, vals []types.Value, n int) *RLEVec {
	return &RLEVec{typ: typ, starts: starts, vals: vals, n: n}
}

func EncodeRLE(src storage.Vector) *RLEVec {
	n := src.Len()
	if n == 0 {
		return NewRLE(src.Type(), nil, nil, 0)
	}
	var starts []int32
	var vals []types.Value
	starts = append(starts, 0)
	vals = append(vals, src.Get(0))
	for i := 1; i < n; i++ {
		cur := src.Get(i)
		prev := vals[len(vals)-1]
		if !equalVal(cur, prev) {
			starts = append(starts, int32(i))
			vals = append(vals, cur)
		}
	}
	return NewRLE(src.Type(), starts, vals, n)
}

func AvgRun(src storage.Vector) float64 {
	n := src.Len()
	if n == 0 {
		return 0
	}
	runs := 1
	prev := src.Get(0)
	for i := 1; i < n; i++ {
		cur := src.Get(i)
		if !equalVal(cur, prev) {
			runs++
			prev = cur
		}
	}
	return float64(n) / float64(runs)
}

func (v *RLEVec) Type() types.DataType     { return v.typ }
func (v *RLEVec) Encoding() types.Encoding { return types.RLE }
func (v *RLEVec) Len() int                 { return v.n }
func (v *RLEVec) Starts() []int32          { return v.starts }
func (v *RLEVec) Vals() []types.Value      { return v.vals }
func (v *RLEVec) runOf(i int) int {
	j := sort.Search(len(v.starts), func(k int) bool { return int(v.starts[k]) > i }) - 1
	if j < 0 {
		j = 0
	}
	return j
}
func (v *RLEVec) IsNull(i int) bool { return v.Get(i).Null }
func (v *RLEVec) NullCount() int {
	c := 0
	for i, val := range v.vals {
		if !val.Null {
			continue
		}
		end := v.n
		if i+1 < len(v.starts) {
			end = int(v.starts[i+1])
		}
		c += end - int(v.starts[i])
	}
	return c
}
func (v *RLEVec) Get(i int) types.Value { return v.vals[v.runOf(i)] }
func (v *RLEVec) RawBytes() int64       { return int64(v.n * 8) }
func (v *RLEVec) MemBytes() int64       { return int64(len(v.starts)*4 + len(v.vals)*16) }
func (v *RLEVec) Take(sel []int) storage.Vector {
	b := storage.NewBuilder(v.typ)
	for _, s := range sel {
		b.Append(v.Get(s))
	}
	return b.Finish()
}

func equalVal(a, b types.Value) bool {
	if a.Null && b.Null {
		return true
	}
	if a.Null || b.Null || a.Type != b.Type {
		return false
	}
	return types.Compare(a, b) == 0
}
