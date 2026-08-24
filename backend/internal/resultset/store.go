package resultset

import (
	"container/list"
	"sync"
	"time"

	"minidatalake/internal/apperr"
	"minidatalake/internal/clock"
	"minidatalake/internal/exec"
	"minidatalake/internal/types"
)

type Item struct {
	ID        string
	SQL       string
	Created   time.Time
	Res       *exec.Result
	Bytes     int64
	ElapsedMS int64

	refs int  // active export readers, protected by Store.mu
	dead bool // tombstone: logically deleted, pending physical removal
}

type Store struct {
	mu   sync.Mutex
	ttl  time.Duration
	max  int
	data map[string]*list.Element
	lru  *list.List
}

func New(ttl time.Duration, max int) *Store {
	if max <= 0 {
		max = 32
	}
	s := &Store{ttl: ttl, max: max, data: map[string]*list.Element{}, lru: list.New()}
	go s.gc()
	return s
}

func (s *Store) Put(it *Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if el, ok := s.data[it.ID]; ok {
		old := el.Value.(*Item)
		if old.refs > 0 {
			// An export of the previous result is still in flight; defer the
			// replacement so we don't swap the pointer out from under it. The
			// caller already holds a separate *Item, so dropping this Put is
			// acceptable for the "re-run same query" path that allocates a new
			// ID each time.
			return
		}
		s.lru.MoveToFront(el)
		el.Value = it
		return
	}
	el := s.lru.PushFront(it)
	s.data[it.ID] = el
	for s.lru.Len() > s.max {
		s.evictBack()
	}
}

// Get returns the item for id. It must be paired with a call to Release
// once the caller is done reading the item's data.
func (s *Store) Get(id string) (*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.data[id]
	if !ok {
		return nil, apperr.New(apperr.ResultExpired, 410, "result set expired or unknown; re-run the query")
	}
	it := el.Value.(*Item)
	if s.ttl > 0 && clock.Now().Sub(it.Created) > s.ttl {
		s.remove(el)
		return nil, apperr.New(apperr.ResultExpired, 410, "result set expired; re-run the query")
	}
	s.lru.MoveToFront(el)
	it.refs++
	return it, nil
}

// Release marks an item's reader as done. A logically-deleted item whose last
// reader releases here is physically removed, freeing its memory.
func (s *Store) Release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.data[id]
	if !ok {
		return
	}
	it := el.Value.(*Item)
	if it.refs > 0 {
		it.refs--
	}
	if it.dead && it.refs == 0 {
		s.remove(el)
	}
}

// Delete logically removes the item. If an export is still streaming the
// result the physical removal is deferred until the last reader releases,
// guaranteeing an in-flight export never sees a nil pointer for Res. The
// boolean reports whether removal was deferred due to an active reader.
func (s *Store) Delete(id string) (deferred bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.data[id]
	if !ok {
		return false
	}
	it := el.Value.(*Item)
	if it.refs > 0 {
		it.dead = true
		return true
	}
	s.remove(el)
	return false
}

func (s *Store) Stats() (n int, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for e := s.lru.Front(); e != nil; e = e.Next() {
		it := e.Value.(*Item)
		n++
		bytes += it.Bytes
	}
	return
}

func (s *Store) evictBack() {
	el := s.lru.Back()
	if el != nil {
		it := el.Value.(*Item)
		if it.refs > 0 {
			it.dead = true // defer until last reader releases
			return
		}
		s.remove(el)
	}
}

// remove physically removes the item from the store. It must only be called
// when no reader is active (refs == 0). Callers that cannot guarantee this
// must check refs and mark dead instead.
func (s *Store) remove(el *list.Element) {
	it := el.Value.(*Item)
	delete(s.data, it.ID)
	s.lru.Remove(el)
}

func (s *Store) gc() {
	t := time.NewTicker(30 * time.Second)
	for range t.C {
		s.mu.Lock()
		now := clock.Now()
		for e := s.lru.Back(); e != nil; {
			prev := e.Prev()
			it := e.Value.(*Item)
			if s.ttl > 0 && now.Sub(it.Created) > s.ttl {
				if it.refs > 0 {
					it.dead = true // keep data alive until the in-flight export finishes
				} else {
					s.remove(e)
				}
			}
			e = prev
		}
		s.mu.Unlock()
	}
}

func Page(it *Item, offset, limit int) [][]any {
	res := it.Res
	if offset < 0 {
		offset = 0
	}
	if offset > res.Rows {
		offset = res.Rows
	}
	end := offset + limit
	if end > res.Rows {
		end = res.Rows
	}
	out := make([][]any, 0, end-offset)
	for r := offset; r < end; r++ {
		row := make([]any, len(res.Cols))
		for c, col := range res.Cols {
			v := col.Get(r)
			row[c] = jsonVal(v)
		}
		out = append(out, row)
	}
	return out
}

func JSONVal(v types.Value) any { return jsonVal(v) }

func jsonVal(v types.Value) any {
	if v.Null {
		return nil
	}
	switch v.Type {
	case types.Int64:
		return v.I
	case types.Float64:
		return v.F
	case types.Bool:
		return v.B
	case types.Timestamp:
		return v.String()
	default:
		return v.S
	}
}

func Schema(it *Item) []map[string]string {
	out := make([]map[string]string, len(it.Res.Names))
	for i, n := range it.Res.Names {
		t := types.String
		if i < len(it.Res.Types) {
			t = it.Res.Types[i]
		}
		out[i] = map[string]string{"name": n, "type": t.String()}
	}
	return out
}
