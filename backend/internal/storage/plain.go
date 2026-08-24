package storage

import (
	"minidatalake/internal/types"
)

type Int64Vec struct {
	vals []int64
	null *Bitmap
}

func NewInt64(vals []int64, null *Bitmap) *Int64Vec {
	if null == nil {
		null = NewBitmap(len(vals))
	}
	return &Int64Vec{vals: vals, null: null}
}

func (v *Int64Vec) Type() types.DataType       { return types.Int64 }
func (v *Int64Vec) Encoding() types.Encoding   { return types.Plain }
func (v *Int64Vec) Len() int                   { return len(v.vals) }
func (v *Int64Vec) IsNull(i int) bool          { return v.null.Get(i) }
func (v *Int64Vec) NullCount() int             { return v.null.Count() }
func (v *Int64Vec) Vals() []int64              { return v.vals }
func (v *Int64Vec) Nulls() *Bitmap             { return v.null }
func (v *Int64Vec) RawBytes() int64            { return int64(len(v.vals) * 8) }
func (v *Int64Vec) MemBytes() int64            { return v.RawBytes() + int64(len(v.null.Bytes())) }
func (v *Int64Vec) Get(i int) types.Value {
	if v.null.Get(i) {
		return types.Null(types.Int64)
	}
	return types.VInt(v.vals[i])
}
func (v *Int64Vec) Take(sel []int) Vector {
	out := make([]int64, len(sel))
	nb := NewBitmap(len(sel))
	for i, s := range sel {
		out[i] = v.vals[s]
		if v.null.Get(s) {
			nb.Set(i)
		}
	}
	return NewInt64(out, nb)
}

type Float64Vec struct {
	vals []float64
	null *Bitmap
}

func NewFloat64(vals []float64, null *Bitmap) *Float64Vec {
	if null == nil {
		null = NewBitmap(len(vals))
	}
	return &Float64Vec{vals: vals, null: null}
}

func (v *Float64Vec) Type() types.DataType       { return types.Float64 }
func (v *Float64Vec) Encoding() types.Encoding   { return types.Plain }
func (v *Float64Vec) Len() int                   { return len(v.vals) }
func (v *Float64Vec) IsNull(i int) bool          { return v.null.Get(i) }
func (v *Float64Vec) NullCount() int             { return v.null.Count() }
func (v *Float64Vec) Vals() []float64            { return v.vals }
func (v *Float64Vec) Nulls() *Bitmap             { return v.null }
func (v *Float64Vec) RawBytes() int64            { return int64(len(v.vals) * 8) }
func (v *Float64Vec) MemBytes() int64            { return v.RawBytes() + int64(len(v.null.Bytes())) }
func (v *Float64Vec) Get(i int) types.Value {
	if v.null.Get(i) {
		return types.Null(types.Float64)
	}
	return types.VFloat(v.vals[i])
}
func (v *Float64Vec) Take(sel []int) Vector {
	out := make([]float64, len(sel))
	nb := NewBitmap(len(sel))
	for i, s := range sel {
		out[i] = v.vals[s]
		if v.null.Get(s) {
			nb.Set(i)
		}
	}
	return NewFloat64(out, nb)
}

type BoolVec struct {
	vals []uint8
	null *Bitmap
}

func NewBool(vals []uint8, null *Bitmap) *BoolVec {
	if null == nil {
		null = NewBitmap(len(vals))
	}
	return &BoolVec{vals: vals, null: null}
}

func (v *BoolVec) Type() types.DataType       { return types.Bool }
func (v *BoolVec) Encoding() types.Encoding   { return types.Plain }
func (v *BoolVec) Len() int                   { return len(v.vals) }
func (v *BoolVec) IsNull(i int) bool          { return v.null.Get(i) }
func (v *BoolVec) NullCount() int             { return v.null.Count() }
func (v *BoolVec) Vals() []uint8              { return v.vals }
func (v *BoolVec) Nulls() *Bitmap             { return v.null }
func (v *BoolVec) RawBytes() int64            { return int64(len(v.vals)) }
func (v *BoolVec) MemBytes() int64            { return v.RawBytes() + int64(len(v.null.Bytes())) }
func (v *BoolVec) Get(i int) types.Value {
	if v.null.Get(i) {
		return types.Null(types.Bool)
	}
	return types.VBool(v.vals[i] != 0)
}
func (v *BoolVec) Take(sel []int) Vector {
	out := make([]uint8, len(sel))
	nb := NewBitmap(len(sel))
	for i, s := range sel {
		out[i] = v.vals[s]
		if v.null.Get(s) {
			nb.Set(i)
		}
	}
	return NewBool(out, nb)
}

type TimeVec struct {
	vals []int64
	null *Bitmap
}

func NewTime(vals []int64, null *Bitmap) *TimeVec {
	if null == nil {
		null = NewBitmap(len(vals))
	}
	return &TimeVec{vals: vals, null: null}
}

func (v *TimeVec) Type() types.DataType       { return types.Timestamp }
func (v *TimeVec) Encoding() types.Encoding   { return types.Plain }
func (v *TimeVec) Len() int                   { return len(v.vals) }
func (v *TimeVec) IsNull(i int) bool          { return v.null.Get(i) }
func (v *TimeVec) NullCount() int             { return v.null.Count() }
func (v *TimeVec) Vals() []int64              { return v.vals }
func (v *TimeVec) Nulls() *Bitmap             { return v.null }
func (v *TimeVec) RawBytes() int64            { return int64(len(v.vals) * 8) }
func (v *TimeVec) MemBytes() int64            { return v.RawBytes() + int64(len(v.null.Bytes())) }
func (v *TimeVec) Get(i int) types.Value {
	if v.null.Get(i) {
		return types.Null(types.Timestamp)
	}
	return types.VTime(v.vals[i])
}
func (v *TimeVec) Take(sel []int) Vector {
	out := make([]int64, len(sel))
	nb := NewBitmap(len(sel))
	for i, s := range sel {
		out[i] = v.vals[s]
		if v.null.Get(s) {
			nb.Set(i)
		}
	}
	return NewTime(out, nb)
}

// StrVec stores strings in a single arena with offsets (not []string).
type StrVec struct {
	data    []byte
	offsets []int32 // n+1
	null    *Bitmap
}

func NewStr(data []byte, offsets []int32, null *Bitmap) *StrVec {
	n := len(offsets) - 1
	if n < 0 {
		n = 0
		offsets = []int32{0}
	}
	if null == nil {
		null = NewBitmap(n)
	}
	return &StrVec{data: data, offsets: offsets, null: null}
}

func BuildStr(ss []string, nulls []bool) *StrVec {
	var data []byte
	off := make([]int32, 0, len(ss)+1)
	off = append(off, 0)
	nb := NewBitmap(len(ss))
	for i, s := range ss {
		data = append(data, s...)
		off = append(off, int32(len(data)))
		if i < len(nulls) && nulls[i] {
			nb.Set(i)
		}
	}
	return NewStr(data, off, nb)
}

func (v *StrVec) Type() types.DataType       { return types.String }
func (v *StrVec) Encoding() types.Encoding   { return types.Plain }
func (v *StrVec) Len() int                   { return len(v.offsets) - 1 }
func (v *StrVec) IsNull(i int) bool          { return v.null.Get(i) }
func (v *StrVec) NullCount() int             { return v.null.Count() }
func (v *StrVec) Data() []byte               { return v.data }
func (v *StrVec) Offsets() []int32           { return v.offsets }
func (v *StrVec) Nulls() *Bitmap             { return v.null }
func (v *StrVec) RawBytes() int64            { return int64(len(v.data) + 4*len(v.offsets)) }
func (v *StrVec) MemBytes() int64            { return v.RawBytes() + int64(len(v.null.Bytes())) }
func (v *StrVec) At(i int) string {
	return string(v.data[v.offsets[i]:v.offsets[i+1]])
}
func (v *StrVec) Get(i int) types.Value {
	if v.null.Get(i) {
		return types.Null(types.String)
	}
	return types.VStr(v.At(i))
}
func (v *StrVec) Take(sel []int) Vector {
	var data []byte
	off := v.offsets[:0]
	off = append(off, 0)
	nb := NewBitmap(len(sel))
	for i, s := range sel {
		if v.null.Get(s) {
			nb.Set(i)
			off = append(off, int32(len(data)))
			continue
		}
		data = append(data, v.data[v.offsets[s]:v.offsets[s+1]]...)
		off = append(off, int32(len(data)))
	}
	return NewStr(data, off, nb)
}
