package storage

import "minidatalake/internal/types"

type Vector interface {
	Type() types.DataType
	Encoding() types.Encoding
	Len() int
	IsNull(i int) bool
	Get(i int) types.Value
	MemBytes() int64
	NullCount() int
	Take(sel []int) Vector
	RawBytes() int64
}

type ColumnMeta struct {
	Name       string
	Type       types.DataType
	Encoding   types.Encoding
	Rows       int
	Nulls      int
	RawBytes   int64
	EncBytes   int64
	Card       int
	AvgRun     float64
	Reason     string
}

func (m ColumnMeta) Ratio() float64 {
	if m.RawBytes <= 0 {
		return 1
	}
	return float64(m.EncBytes) / float64(m.RawBytes)
}
