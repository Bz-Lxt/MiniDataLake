package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"minidatalake/internal/clock"
)

type JobRec struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Phase        string `json:"phase"`
	FileName     string `json:"file_name"`
	Table        string `json:"table"`
	Format       string `json:"format"`
	Hash         string `json:"hash"`
	BytesTotal   int64  `json:"bytes_total"`
	BytesDone    int64  `json:"bytes_done"`
	RowsDone     int   `json:"rows_done"`
	Error        string `json:"error"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Reused       bool   `json:"reused"`
}

type TableRec struct {
	Name        string `json:"name"`
	SourceFile  string `json:"source_file"`
	Hash        string `json:"hash"`
	Format      string `json:"format"`
	MDLPath     string `json:"mdl_path"`
	Rows        int    `json:"rows"`
	FileBytes   int64  `json:"file_bytes"`
	CreatedAt   string `json:"created_at"`
	Status      string `json:"status"`
	Rejected    int    `json:"rejected"`
}

type Manifest struct {
	Tables []TableRec `json:"tables"`
	Jobs   []JobRec   `json:"jobs"`
}

type Store struct {
	path string
	mu   sync.Mutex
	m    Manifest
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "manifest.json")}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.m)
	}
	changed := false
	for i := range s.m.Jobs {
		if s.m.Jobs[i].Status == "RUNNING" {
			s.m.Jobs[i].Status = "INTERRUPTED"
			s.m.Jobs[i].Phase = "interrupted"
			s.m.Jobs[i].UpdatedAt = clock.Format(clock.Now())
			changed = true
		}
	}
	if changed {
		_ = s.flush()
	}
	return s, nil
}

func (s *Store) Snapshot() Manifest {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := Manifest{Tables: append([]TableRec{}, s.m.Tables...), Jobs: append([]JobRec{}, s.m.Jobs...)}
	return cp
}

func (s *Store) UpsertTable(t TableRec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for i, x := range s.m.Tables {
		if x.Name == t.Name {
			s.m.Tables[i] = t
			found = true
			break
		}
	}
	if !found {
		s.m.Tables = append(s.m.Tables, t)
	}
	return s.flush()
}

func (s *Store) DeleteTable(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.m.Tables[:0]
	for _, t := range s.m.Tables {
		if t.Name != name {
			out = append(out, t)
		}
	}
	s.m.Tables = out
	return s.flush()
}

func (s *Store) FindHash(h string) (TableRec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.m.Tables {
		if t.Hash == h && t.Status == "READY" {
			return t, true
		}
	}
	return TableRec{}, false
}

func (s *Store) UpsertJob(j JobRec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.m.Jobs {
		if x.ID == j.ID {
			s.m.Jobs[i] = j
			return s.flush()
		}
	}
	s.m.Jobs = append(s.m.Jobs, j)
	return s.flush()
}

func (s *Store) Job(id string) (JobRec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.m.Jobs {
		if j.ID == id {
			return j, true
		}
	}
	return JobRec{}, false
}

func (s *Store) flush() error {
	b, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
