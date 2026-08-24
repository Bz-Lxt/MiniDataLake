package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/logx"
)

func TestHealth(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.DataDir = dir
	cfg.StaticDir = dir
	eng, err := app.New(cfg, logx.New("error"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Eng: eng, Log: logx.New("error")}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_ = os.WriteFile(filepath.Join(dir, "ok"), []byte("1"), 0644)
}
