package memgov

import (
	"sort"
	"sync/atomic"

	"minidatalake/internal/apperr"
)

type Budget struct {
	limit int64
	used  atomic.Int64
	hint  func() []Victim
}

type Victim struct {
	Name  string
	Bytes int64
}

func New(limit int64, hint func() []Victim) *Budget {
	if limit <= 0 {
		limit = 1 << 30
	}
	return &Budget{limit: limit, hint: hint}
}

func (b *Budget) Limit() int64 { return b.limit }
func (b *Budget) Used() int64  { return b.used.Load() }

func (b *Budget) Reserve(n int64) error {
	if n <= 0 {
		return nil
	}
	for {
		cur := b.used.Load()
		if cur+n > b.limit {
			return b.deny(n)
		}
		if b.used.CompareAndSwap(cur, cur+n) {
			return nil
		}
	}
}

func (b *Budget) Release(n int64) {
	if n <= 0 {
		return
	}
	b.used.Add(-n)
}

func (b *Budget) deny(need int64) error {
	e := apperr.Mem("memory budget exceeded").
		With("need_bytes", need).
		With("used_bytes", b.used.Load()).
		With("limit_bytes", b.limit)
	if b.hint != nil {
		vs := b.hint()
		sort.Slice(vs, func(i, j int) bool { return vs[i].Bytes > vs[j].Bytes })
		names := make([]string, 0, 3)
		for i := 0; i < len(vs) && i < 3; i++ {
			names = append(names, vs[i].Name)
		}
		e.With("suggest_unload", names)
	}
	return e
}

func EstimateIngest(fileBytes int64) int64 {
	if fileBytes < 1 {
		return 8 << 20
	}
	est := fileBytes * 3
	if est < 8<<20 {
		est = 8 << 20
	}
	return est
}
