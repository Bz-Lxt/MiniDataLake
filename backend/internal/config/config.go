package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr              string
	DataDir           string
	StaticDir         string
	LogLevel          string
	APIToken          string
	MemoryBudgetBytes int64
	MaxUploadBytes    int64
	ResultTTLSeconds  int
	PageDefault       int
	PageMax           int
	BatchSize         int
	ChunkBytes        int
	QueryTimeoutSec   int
	DictCardRatio     float64
	RLEMinRun         float64
	MaxNestedJSON     int
}

func Load() Config {
	return Config{
		Addr:              env("MDL_ADDR", ":8080"),
		DataDir:           env("MDL_DATA_DIR", "./data"),
		StaticDir:         env("MDL_STATIC_DIR", "./static"),
		LogLevel:          env("MDL_LOG_LEVEL", "info"),
		APIToken:          os.Getenv("MDL_API_TOKEN"),
		MemoryBudgetBytes: envI64("MDL_MEMORY_BUDGET_BYTES", 1610612736),
		MaxUploadBytes:    envI64("MDL_MAX_UPLOAD_BYTES", 134217728),
		ResultTTLSeconds:  envInt("MDL_RESULT_TTL_SECONDS", 600),
		PageDefault:       envInt("MDL_PAGE_DEFAULT", 1000),
		PageMax:           envInt("MDL_PAGE_MAX", 10000),
		BatchSize:         envInt("MDL_BATCH_SIZE", 4096),
		ChunkBytes:        envInt("MDL_CHUNK_BYTES", 1<<20),
		QueryTimeoutSec:   envInt("MDL_QUERY_TIMEOUT_SEC", 30),
		DictCardRatio:     envF64("MDL_DICT_CARD_RATIO", 0.05),
		RLEMinRun:         envF64("MDL_RLE_MIN_RUN", 8),
		MaxNestedJSON:     envInt("MDL_MAX_NESTED_JSON", 6),
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envI64(key string, def int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envF64(key string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}
