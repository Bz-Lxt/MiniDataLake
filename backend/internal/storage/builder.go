package storage

import "minidatalake/internal/types"

type Builder struct {
	typ  types.DataType
	i64  []int64
	f64  []float64
	b    []uint8
	data []byte
	off  []int32
	null *Bitmap
	n    int
}

func NewBuilder(t types.DataType) *Builder {
	b := &Builder{typ: t, off: []int32{0}, null: NewBitmap(0)}
	return b
}

func (b *Builder) Len() int { return b.n }

func (b *Builder) Append(v types.Value) {
	if v.Null {
		b.AppendNull()
		return
	}
	switch b.typ {
	case types.Int64:
		b.i64 = append(b.i64, v.I)
	case types.Float64:
		b.f64 = append(b.f64, v.F)
	case types.Bool:
		var x uint8
		if v.B {
			x = 1
		}
		b.b = append(b.b, x)
	case types.Timestamp:
		b.i64 = append(b.i64, v.I)
	default:
		b.data = append(b.data, v.S...)
		b.off = append(b.off, int32(len(b.data)))
	}
	b.null.grow(b.n + 1)
	b.n++
}

func (b *Builder) AppendNull() {
	switch b.typ {
	case types.Int64, types.Timestamp:
		b.i64 = append(b.i64, 0)
	case types.Float64:
		b.f64 = append(b.f64, 0)
	case types.Bool:
		b.b = append(b.b, 0)
	default:
		b.off = append(b.off, int32(len(b.data)))
	}
	b.null.Append(true)
	b.n++
}

func (b *Builder) Finish() Vector {
	switch b.typ {
	case types.Int64:
		return NewInt64(b.i64, b.null)
	case types.Float64:
		return NewFloat64(b.f64, b.null)
	case types.Bool:
		return NewBool(b.b, b.null)
	case types.Timestamp:
		return NewTime(b.i64, b.null)
	default:
		return NewStr(b.data, b.off, b.null)
	}
}

func Concat(vs []Vector) Vector {
	if len(vs) == 0 {
		return NewInt64(nil, NewBitmap(0))
	}
	b := NewBuilder(vs[0].Type())
	for _, v := range vs {
		n := v.Len()
		for i := 0; i < n; i++ {
			b.Append(v.Get(i))
		}
	}
	return b.Finish()
}

func FromValues(t types.DataType, vals []types.Value) Vector {
	b := NewBuilder(t)
	for _, v := range vals {
		if v.Null {
			b.AppendNull()
		} else {
			b.Append(v)
		}
	}
	return b.Finish()
}
