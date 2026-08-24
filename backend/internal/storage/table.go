package storage

import (
	"sync"
	"sync/atomic"

	"minidatalake/internal/types"
)

type Column struct {
	Meta ColumnMeta
	Vec  Vector
}

type Table struct {
	Name         string
	SourceFile   string
	ContentHash  string
	Format       string
	Rows         int
	Cols         []*Column
	Rejected     int
	RejectSample []RejectRow
	FileBytes    int64
	CreatedAt    string
	Status       string // READY | CORRUPTED
	mu           sync.RWMutex
	loaded       atomic.Bool
}

type RejectRow struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
	Sample string `json:"sample"`
}

func (t *Table) Schema() []ColumnMeta {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]ColumnMeta, len(t.Cols))
	for i, c := range t.Cols {
		out[i] = c.Meta
	}
	return out
}

func (t *Table) ColByName(name string) (*Column, int, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for i, c := range t.Cols {
		if c.Meta.Name == name {
			return c, i, true
		}
	}
	return nil, -1, false
}

func (t *Table) MemBytes() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var n int64
	for _, c := range t.Cols {
		if c.Vec != nil {
			n += c.Vec.MemBytes()
		} else {
			n += c.Meta.EncBytes
		}
	}
	return n
}

func (t *Table) RawBytes() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var n int64
	for _, c := range t.Cols {
		n += c.Meta.RawBytes
	}
	return n
}

func (t *Table) Preview(n int) [][]types.Value {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if n > t.Rows {
		n = t.Rows
	}
	out := make([][]types.Value, n)
	for r := 0; r < n; r++ {
		row := make([]types.Value, len(t.Cols))
		for c, col := range t.Cols {
			if col.Vec == nil {
				row[c] = types.Null(col.Meta.Type)
				continue
			}
			row[c] = col.Vec.Get(r)
		}
		out[r] = row
	}
	return out
}

func (t *Table) Slice(names []string, start, end int) []Vector {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if end > t.Rows {
		end = t.Rows
	}
	if start < 0 {
		start = 0
	}
	if start >= end {
		out := make([]Vector, len(names))
		for i := range names {
			out[i] = emptyOf(t.colType(names[i]))
		}
		return out
	}
	sel := make([]int, end-start)
	for i := range sel {
		sel[i] = start + i
	}
	out := make([]Vector, len(names))
	for i, name := range names {
		col, _, ok := t.lookup(name)
		if !ok || col.Vec == nil {
			out[i] = emptyOf(types.String)
			continue
		}
		out[i] = col.Vec.Take(sel)
	}
	return out
}

func (t *Table) lookup(name string) (*Column, int, bool) {
	for i, c := range t.Cols {
		if c.Meta.Name == name {
			return c, i, true
		}
	}
	return nil, -1, false
}

func (t *Table) colType(name string) types.DataType {
	if c, _, ok := t.lookup(name); ok {
		return c.Meta.Type
	}
	return types.String
}

func emptyOf(t types.DataType) Vector {
	switch t {
	case types.Int64:
		return NewInt64(nil, NewBitmap(0))
	case types.Float64:
		return NewFloat64(nil, NewBitmap(0))
	case types.Bool:
		return NewBool(nil, NewBitmap(0))
	case types.Timestamp:
		return NewTime(nil, NewBitmap(0))
	default:
		return NewStr(nil, []int32{0}, NewBitmap(0))
	}
}

type RecordBatch struct {
	Names []string
	Cols  []Vector
	Rows  int
}

func NewBatch(names []string, cols []Vector) *RecordBatch {
	n := 0
	if len(cols) > 0 {
		n = cols[0].Len()
	}
	return &RecordBatch{Names: names, Cols: cols, Rows: n}
}

func (b *RecordBatch) Col(name string) (Vector, bool) {
	for i, n := range b.Names {
		if n == name {
			return b.Cols[i], true
		}
	}
	return nil, false
}
