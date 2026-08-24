package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"minidatalake/internal/app"
	"minidatalake/internal/apperr"
	"minidatalake/internal/clock"
	"minidatalake/internal/logx"
	"minidatalake/internal/resultset"
)

type Server struct {
	Eng *app.Engine
	Log *slog.Logger
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/catalog", s.catalog)
	mux.HandleFunc("GET /api/v1/tables/{name}", s.table)
	mux.HandleFunc("GET /api/v1/tables/{name}/preview", s.preview)
	mux.HandleFunc("DELETE /api/v1/tables/{name}", s.drop)
	mux.HandleFunc("POST /api/v1/files", s.upload)
	mux.HandleFunc("GET /api/v1/jobs", s.jobs)
	mux.HandleFunc("GET /api/v1/jobs/{id}", s.job)
	mux.HandleFunc("POST /api/v1/jobs/{id}/retry", s.retry)
	mux.HandleFunc("POST /api/v1/query", s.query)
	mux.HandleFunc("POST /api/v1/query/explain", s.explain)
	mux.HandleFunc("POST /api/v1/query/{id}/cancel", s.cancel)
	mux.HandleFunc("GET /api/v1/results/{id}", s.results)
	mux.HandleFunc("GET /api/v1/results/{id}/export", s.export)
	mux.HandleFunc("DELETE /api/v1/results/{id}", s.delResult)
	mux.HandleFunc("GET /api/v1/system/stats", s.stats)
	mux.HandleFunc("GET /api/v1/history", s.history)
	fs := http.FileServer(http.Dir(s.Eng.Cfg.StaticDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(s.Eng.Cfg.StaticDir, filepath.Clean(r.URL.Path))
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/index") {
			http.ServeFile(w, r, filepath.Join(s.Eng.Cfg.StaticDir, "index.html"))
			return
		}
		if _, err := os.Stat(p); err == nil {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(s.Eng.Cfg.StaticDir, "index.html"))
	})
	return s.wrap(mux)
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = strconv.FormatInt(clock.Now().UnixNano(), 36)
		}
		ctx := logx.WithRequestID(r.Context(), id)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		if tok := strings.TrimSpace(s.Eng.Cfg.APIToken); tok != "" && strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/health" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if got != tok {
				s.fail(w, r, apperr.New(apperr.Unauthorized, 401, "invalid token"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	s.ok(w, map[string]any{"status": "ok", "time": clock.Format(clock.Now())})
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	var nodes []map[string]any
	for _, t := range s.Eng.Cat.List() {
		cols := []map[string]any{}
		for _, c := range t.Schema() {
			cols = append(cols, map[string]any{
				"name": c.Name, "type": c.Type.String(), "encoding": c.Encoding.String(),
				"nulls": c.Nulls, "raw_bytes": c.RawBytes, "enc_bytes": c.EncBytes,
				"compression": c.Ratio(), "reason": c.Reason,
			})
		}
		nodes = append(nodes, map[string]any{
			"file_name": t.SourceFile, "table": t.Name, "format": t.Format,
			"rows": t.Rows, "status": t.Status, "columns": cols,
		})
	}
	if nodes == nil {
		nodes = []map[string]any{}
	}
	s.ok(w, map[string]any{"tables": nodes})
}

func (s *Server) table(w http.ResponseWriter, r *http.Request) {
	t, ok := s.Eng.Cat.Get(r.PathValue("name"))
	if !ok {
		s.fail(w, r, apperr.Miss("table not found"))
		return
	}
	cols := []map[string]any{}
	for _, c := range t.Schema() {
		cols = append(cols, map[string]any{
			"name": c.Name, "type": c.Type.String(), "encoding": c.Encoding.String(),
			"nulls": c.Nulls, "raw_bytes": c.RawBytes, "enc_bytes": c.EncBytes,
			"compression": c.Ratio(), "reason": c.Reason, "card": c.Card, "avg_run": c.AvgRun,
		})
	}
	s.ok(w, map[string]any{
		"name": t.Name, "file_name": t.SourceFile, "rows": t.Rows, "status": t.Status,
		"mem_bytes": t.MemBytes(), "file_bytes": t.FileBytes, "rejected": t.Rejected,
		"reject_samples": t.RejectSample, "created_at": t.CreatedAt, "hash": t.ContentHash,
		"columns": cols, "format": t.Format,
	})
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	t, ok := s.Eng.Cat.Get(r.PathValue("name"))
	if !ok {
		s.fail(w, r, apperr.Miss("table not found"))
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	names := []string{}
	for _, c := range t.Schema() {
		names = append(names, c.Name)
	}
	s.ok(w, map[string]any{"columns": names, "rows": app.Preview(t, n)})
}

func (s *Server) drop(w http.ResponseWriter, r *http.Request) {
	if err := s.Eng.Cat.Delete(r.PathValue("name")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.ok(w, map[string]any{"deleted": r.PathValue("name")})
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.Eng.Cfg.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(s.Eng.Cfg.MaxUploadBytes); err != nil {
		s.fail(w, r, apperr.New(apperr.UploadTooLarge, 413, "upload too large or invalid multipart"))
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		s.fail(w, r, apperr.Bad("missing file field"))
		return
	}
	defer file.Close()
	job, err := s.Eng.StartIngest(hdr.Filename, r.FormValue("format"), hdr.Header.Get("Content-Type"), file, hdr.Size)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ok(w, job)
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	s.ok(w, map[string]any{"jobs": s.Eng.Store.Snapshot().Jobs})
}

func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	j, ok := s.Eng.Store.Job(r.PathValue("id"))
	if !ok {
		s.fail(w, r, apperr.Miss("job not found"))
		return
	}
	s.ok(w, j)
}

func (s *Server) retry(w http.ResponseWriter, r *http.Request) {
	j, err := s.Eng.RetryJob(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ok(w, j)
}

func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.SQL) == "" {
		s.fail(w, r, apperr.Bad("sql is required"))
		return
	}
	it, err := s.Eng.Query(r.Context(), body.SQL)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	page := s.Eng.Cfg.PageDefault
	s.ok(w, map[string]any{
		"result_set_id": it.ID, "schema": resultset.Schema(it), "total_rows": it.Res.Rows,
		"rows": resultset.Page(it, 0, page), "elapsed_ms": it.ElapsedMS, "scanned_rows": it.Res.Scanned,
		"explain": it.Res.Plan, "sql": it.SQL,
	})
}

func (s *Server) explain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.fail(w, r, apperr.Bad("sql is required"))
		return
	}
	it, err := s.Eng.Query(r.Context(), "EXPLAIN "+strings.TrimSpace(body.SQL))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.ok(w, map[string]any{"explain": it.Res.Plan, "scanned_rows": it.Res.Scanned, "total_rows": it.Res.Rows})
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	s.Eng.CancelQuery(r.PathValue("id"))
	s.ok(w, map[string]any{"canceled": r.PathValue("id")})
}

func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	it, err := s.Eng.RS.Get(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 {
		lim = s.Eng.Cfg.PageDefault
	}
	if lim > s.Eng.Cfg.PageMax {
		s.fail(w, r, apperr.Bad("limit exceeds maximum").With("max", s.Eng.Cfg.PageMax))
		return
	}
	if off < 0 {
		s.fail(w, r, apperr.Bad("offset must be >= 0"))
		return
	}
	s.ok(w, map[string]any{
		"result_set_id": it.ID, "schema": resultset.Schema(it), "total_rows": it.Res.Rows,
		"offset": off, "limit": lim, "rows": resultset.Page(it, off, lim),
	})
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	it, err := s.Eng.RS.Get(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=result.json")
		enc := json.NewEncoder(w)
		for i := 0; i < it.Res.Rows; i++ {
			row := map[string]any{}
			for c, n := range it.Res.Names {
				row[n] = resultset.JSONVal(it.Res.Cols[c].Get(i))
			}
			if err := enc.Encode(row); err != nil {
				return
			}
		}
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=result.csv")
	_, _ = io.WriteString(w, strings.Join(it.Res.Names, ",")+"\n")
	for i := 0; i < it.Res.Rows; i++ {
		parts := make([]string, len(it.Res.Names))
		for c := range it.Res.Names {
			v := it.Res.Cols[c].Get(i)
			if v.Null {
				parts[c] = ""
			} else {
				parts[c] = escapeCSV(v.String())
			}
		}
		_, _ = io.WriteString(w, strings.Join(parts, ",")+"\n")
	}
}

func (s *Server) delResult(w http.ResponseWriter, r *http.Request) {
	s.Eng.RS.Delete(r.PathValue("id"))
	s.ok(w, map[string]any{"deleted": r.PathValue("id")})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	s.ok(w, s.Eng.SystemStats())
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	s.ok(w, map[string]any{"note": "query history is stored in the browser"})
}

func escapeCSV(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

func (s *Server) ok(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	e, ok := apperr.As(err)
	if !ok {
		e = apperr.New(apperr.Internal, 500, err.Error())
	}
	s.Log.Error("request failed", "code", e.Code, "msg", e.Message, "request_id", logx.RequestID(r.Context()))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code": e.Code, "message": e.Message, "details": e.Details, "request_id": logx.RequestID(r.Context()),
	})
}
