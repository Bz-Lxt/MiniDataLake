package catalog

import (
	"path/filepath"
	"sync"

	"minidatalake/internal/memgov"
	"minidatalake/internal/persist"
	"minidatalake/internal/storage"
)

type Catalog struct {
	mu    sync.RWMutex
	tables map[string]*storage.Table
	store *persist.Store
	dir   string
	bud   *memgov.Budget
}

func Open(dir string, store *persist.Store, bud *memgov.Budget) (*Catalog, error) {
	c := &Catalog{tables: map[string]*storage.Table{}, store: store, dir: dir, bud: bud}
	snap := store.Snapshot()
	for _, rec := range snap.Tables {
		t, err := persist.ReadTable(rec.MDLPath)
		if err != nil {
			t = &storage.Table{
				Name: rec.Name, SourceFile: rec.SourceFile, ContentHash: rec.Hash,
				Format: rec.Format, Rows: rec.Rows, FileBytes: rec.FileBytes,
				CreatedAt: rec.CreatedAt, Status: "CORRUPTED",
			}
			c.tables[rec.Name] = t
			continue
		}
		t.Name = rec.Name
		t.SourceFile = rec.SourceFile
		t.ContentHash = rec.Hash
		t.Format = rec.Format
		t.FileBytes = rec.FileBytes
		t.CreatedAt = rec.CreatedAt
		t.Rejected = rec.Rejected
		t.Status = rec.Status
		if t.Status == "" {
			t.Status = "READY"
		}
		c.tables[rec.Name] = t
		if bud != nil {
			_ = bud.Reserve(t.MemBytes())
		}
	}
	return c, nil
}

func (c *Catalog) Put(t *storage.Table) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.tables[t.Name]; ok && c.bud != nil {
		c.bud.Release(old.MemBytes())
	}
	c.tables[t.Name] = t
	path := filepath.Join(c.dir, t.Name+".mdl")
	if err := persist.WriteTable(path, t); err != nil {
		return err
	}
	return c.store.UpsertTable(persist.TableRec{
		Name: t.Name, SourceFile: t.SourceFile, Hash: t.ContentHash, Format: t.Format,
		MDLPath: path, Rows: t.Rows, FileBytes: t.FileBytes, CreatedAt: t.CreatedAt,
		Status: t.Status, Rejected: t.Rejected,
	})
}

func (c *Catalog) Get(name string) (*storage.Table, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.tables[name]
	return t, ok
}

func (c *Catalog) List() []*storage.Table {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*storage.Table, 0, len(c.tables))
	for _, t := range c.tables {
		out = append(out, t)
	}
	return out
}

func (c *Catalog) Names() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := map[string]bool{}
	for k := range c.tables {
		m[k] = true
	}
	return m
}

func (c *Catalog) Delete(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tables[name]
	if !ok {
		return nil
	}
	if c.bud != nil {
		c.bud.Release(t.MemBytes())
	}
	delete(c.tables, name)
	path := filepath.Join(c.dir, name+".mdl")
	_ = persist.DeleteFile(path)
	return c.store.DeleteTable(name)
}

func (c *Catalog) Victims() []memgov.Victim {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var vs []memgov.Victim
	for _, t := range c.tables {
		vs = append(vs, memgov.Victim{Name: t.Name, Bytes: t.MemBytes()})
	}
	return vs
}
