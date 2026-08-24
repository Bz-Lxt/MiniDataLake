package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"minidatalake/internal/app"
	"minidatalake/internal/config"
	"minidatalake/internal/httpapi"
	"minidatalake/internal/logx"
)

func main() {
	cfg := config.Load()
	log := logx.New(cfg.LogLevel)
	eng, err := app.New(cfg, log)
	if err != nil {
		log.Error("init failed", "err", err)
		os.Exit(1)
	}
	srv := &httpapi.Server{Eng: eng, Log: log}
	hs := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      10 * time.Minute,
	}
	log.Info("MiniDataLake listening", "addr", cfg.Addr, "data", cfg.DataDir, slog.String("tz", "Asia/Shanghai"))
	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server exit", "err", err)
		os.Exit(1)
	}
}
