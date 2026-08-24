package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"minidatalake/internal/apperr"
	"minidatalake/internal/catalog"
	"minidatalake/internal/clock"
	"minidatalake/internal/config"
	"minidatalake/internal/exec"
	"minidatalake/internal/ingest"
	"minidatalake/internal/memgov"
	"minidatalake/internal/persist"
	"minidatalake/internal/resultset"
	"minidatalake/internal/sqlplan"
	"minidatalake/internal/storage"
)

type Engine struct {
	Cfg   config.Config
	Log   *slog.Logger
	Cat   *catalog.Catalog
	Store *persist.Store
	Bud   *memgov.Budget
	RS    *resultset.Store
	mu    sync.Mutex
	jobs  map[string]context.CancelFunc
	qcan  map[string]context.CancelFunc
}

func New(cfg config.Config, log *slog.Logger) (*Engine, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	st, err := persist.OpenStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	e := &Engine{Cfg: cfg, Log: log, Store: st, jobs: map[string]context.CancelFunc{}, qcan: map[string]context.CancelFunc{}}
	e.Bud = memgov.New(cfg.MemoryBudgetBytes, func() []memgov.Victim {
		if e.Cat == nil {
			return nil
		}
		return e.Cat.Victims()
	})
	cat, err := catalog.Open(cfg.DataDir, st, e.Bud)
	if err != nil {
		return nil, err
	}
	e.Cat = cat
	e.RS = resultset.New(time.Duration(cfg.ResultTTLSeconds)*time.Second, 48)
	return e, nil
}

func (e *Engine) StartIngest(filename, format, contentType string, src io.Reader, size int64) (persist.JobRec, error) {
	if format == "" {
		format = ingest.DetectFormat(filename, contentType)
	}
	if format == "" {
		return persist.JobRec{}, apperr.New(apperr.UnsupportedFormat, 400, "unsupported file type")
	}
	if size > e.Cfg.MaxUploadBytes {
		return persist.JobRec{}, apperr.New(apperr.UploadTooLarge, 413, "file exceeds upload limit")
	}
	if err := e.Bud.Reserve(memgov.EstimateIngest(size)); err != nil {
		return persist.JobRec{}, err
	}

	id := newID()
	tmp := filepath.Join(e.Cfg.DataDir, "incoming", id+"-"+filepath.Base(filename))
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		e.Bud.Release(memgov.EstimateIngest(size))
		return persist.JobRec{}, err
	}
	f, err := os.Create(tmp)
	if err != nil {
		e.Bud.Release(memgov.EstimateIngest(size))
		return persist.JobRec{}, err
	}
	n, err := io.Copy(f, src)
	f.Close()
	if err != nil {
		e.Bud.Release(memgov.EstimateIngest(size))
		return persist.JobRec{}, err
	}

	job := persist.JobRec{
		ID: id, Status: "RUNNING", Phase: "queued", FileName: filename,
		Format: format, BytesTotal: n, CreatedAt: clock.Format(clock.Now()), UpdatedAt: clock.Format(clock.Now()),
	}
	_ = e.Store.UpsertJob(job)
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.jobs[id] = cancel
	e.mu.Unlock()
	go e.runIngest(ctx, job, tmp)
	return job, nil
}

func (e *Engine) runIngest(ctx context.Context, job persist.JobRec, path string) {
	defer func() {
		if rec := recover(); rec != nil {
			job.Status = "FAILED"
			job.Phase = "panic"
			job.Error = "ingest panic: " + toString(rec)
			job.UpdatedAt = clock.Format(clock.Now())
			_ = e.Store.UpsertJob(job)
			e.Log.Error("ingest panic recovered", "job", job.ID, "err", job.Error)
		}
		e.mu.Lock()
		delete(e.jobs, job.ID)
		e.mu.Unlock()
		e.Bud.Release(memgov.EstimateIngest(job.BytesTotal))
	}()
	upd := func(j persist.JobRec) {
		j.UpdatedAt = clock.Format(clock.Now())
		_ = e.Store.UpsertJob(j)
		job = j
	}
	sum, err := fileHash(path)
	if err != nil {
		job.Status = "FAILED"
		job.Error = err.Error()
		upd(job)
		return
	}
	job.Hash = sum
	if rec, ok := e.Store.FindHash(sum); ok {
		job.Status = "DONE"
		job.Phase = "reused"
		job.Table = rec.Name
		job.Reused = true
		job.RowsDone = rec.Rows
		job.BytesDone = job.BytesTotal
		upd(job)
		return
	}
	t, err := ingest.Ingest(ctx, path, ingest.Options{
		Filename: job.FileName, Format: job.Format, Cfg: e.Cfg, Log: e.Log,
		OnProg: func(p ingest.Progress) {
			job.Phase = p.Phase
			job.BytesDone = p.BytesDone
			job.RowsDone = p.Rows
			job.Status = "RUNNING"
			upd(job)
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			job.Status = "INTERRUPTED"
			job.Error = "canceled"
		} else {
			job.Status = "FAILED"
			job.Error = err.Error()
		}
		upd(job)
		return
	}
	taken := e.Cat.Names()
	t.Name = ingest.TableName(job.FileName, taken)
	if err := e.Bud.Reserve(t.MemBytes()); err != nil {
		job.Status = "FAILED"
		job.Error = err.Error()
		upd(job)
		return
	}
	if err := e.Cat.Put(t); err != nil {
		e.Bud.Release(t.MemBytes())
		job.Status = "FAILED"
		job.Error = err.Error()
		upd(job)
		return
	}
	job.Status = "DONE"
	job.Phase = "ready"
	job.Table = t.Name
	job.RowsDone = t.Rows
	job.BytesDone = job.BytesTotal
	upd(job)
}

func (e *Engine) RetryJob(id string) (persist.JobRec, error) {
	j, ok := e.Store.Job(id)
	if !ok {
		return persist.JobRec{}, apperr.Miss("job not found")
	}
	if j.Status != "INTERRUPTED" && j.Status != "FAILED" {
		return persist.JobRec{}, apperr.Bad("job is not retryable")
	}
	path := filepath.Join(e.Cfg.DataDir, "incoming", id+"-"+filepath.Base(j.FileName))
	if _, err := os.Stat(path); err != nil {
		return persist.JobRec{}, apperr.Miss("original upload no longer available; re-upload the file")
	}
	j.Status = "RUNNING"
	j.Phase = "retry"
	j.Error = ""
	_ = e.Store.UpsertJob(j)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.mu.Lock()
	e.jobs[id] = cancel
	e.mu.Unlock()
	go e.runIngest(ctx, j, path)
	return j, nil
}

func (e *Engine) Query(ctx context.Context, sql string) (*resultset.Item, error) {
	pl, err := sqlplan.Parse(sql)
	if err != nil {
		return nil, err
	}
	t, ok := e.Cat.Get(pl.Table)
	if !ok {
		return nil, apperr.Miss("table not found: " + pl.Table).With("hint", suggestTable(e.Cat, pl.Table))
	}
	if err := sqlplan.Bind(pl, t); err != nil {
		return nil, err
	}
	if pl.Explain {
		res, err := exec.Run(ctx, t, pl, e.Cfg.BatchSize)
		if err != nil {
			return nil, err
		}
		id := newID()
		it := &resultset.Item{ID: id, SQL: sql, Created: clock.Now(), Res: res}
		e.RS.Put(it)
		return it, nil
	}
	qctx, cancel := context.WithTimeout(ctx, time.Duration(e.Cfg.QueryTimeoutSec)*time.Second)
	qid := newID()
	e.mu.Lock()
	e.qcan[qid] = cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		delete(e.qcan, qid)
		e.mu.Unlock()
	}()
	start := time.Now()
	res, err := exec.Run(qctx, t, pl, e.Cfg.BatchSize)
	if err != nil {
		if qctx.Err() == context.DeadlineExceeded {
			return nil, apperr.New(apperr.QueryTimeout, 408, "query timeout")
		}
		if qctx.Err() == context.Canceled {
			return nil, apperr.New(apperr.QueryCanceled, 499, "query canceled")
		}
		return nil, err
	}
	it := &resultset.Item{
		ID: qid, SQL: sql, Created: clock.Now(), Res: res,
		ElapsedMS: time.Since(start).Milliseconds(), Bytes: resBytes(res),
	}
	e.RS.Put(it)
	return it, nil
}

func (e *Engine) CancelQuery(id string) {
	e.mu.Lock()
	if c, ok := e.qcan[id]; ok {
		c()
	}
	e.mu.Unlock()
}

func (e *Engine) SystemStats() map[string]any {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	tables := e.Cat.List()
	var rows int
	var mem int64
	var raw int64
	items := []map[string]any{}
	for _, t := range tables {
		rows += t.Rows
		mem += t.MemBytes()
		raw += t.RawBytes()
		items = append(items, map[string]any{
			"name": t.Name, "rows": t.Rows, "mem_bytes": t.MemBytes(), "status": t.Status,
		})
	}
	rsN, rsB := e.RS.Stats()
	ratio := 1.0
	if raw > 0 {
		ratio = float64(mem) / float64(raw)
	}
	return map[string]any{
		"go_heap_alloc": ms.Alloc, "go_sys": ms.Sys, "num_goroutine": runtime.NumGoroutine(),
		"table_count": len(tables), "total_rows": rows, "tables_mem_bytes": mem,
		"tables_raw_bytes": raw, "global_compression": ratio,
		"resultset_count": rsN, "resultset_bytes": rsB,
		"budget_used": e.Bud.Used(), "budget_limit": e.Bud.Limit(),
		"tables": items, "time": clock.Format(clock.Now()),
	}
}

func resBytes(r *exec.Result) int64 {
	var n int64
	for _, c := range r.Cols {
		n += c.MemBytes()
	}
	return n
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func suggestTable(c *catalog.Catalog, name string) string {
	low := strings.ToLower(name)
	for _, t := range c.List() {
		if strings.Contains(strings.ToLower(t.Name), low) || strings.Contains(low, strings.ToLower(t.Name)) {
			return t.Name
		}
	}
	return ""
}

func Preview(t *storage.Table, n int) [][]any {
	if n <= 0 {
		n = 20
	}
	vals := t.Preview(n)
	out := make([][]any, len(vals))
	for i, row := range vals {
		r := make([]any, len(row))
		for j, v := range row {
			r[j] = resultset.JSONVal(v)
		}
		out[i] = r
	}
	return out
}

func toString(v any) string {
	return fmt.Sprint(v)
}
