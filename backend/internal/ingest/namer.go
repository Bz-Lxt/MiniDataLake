package ingest

import (
	"path/filepath"
	"strconv"
	"strings"

	"minidatalake/internal/types"
)

func TableName(filename string, taken map[string]bool) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	name := types.Ident(stem)
	if name == "col" {
		name = "t"
	}
	if !taken[name] {
		return name
	}
	for i := 2; ; i++ {
		cand := name + "_" + strconv.Itoa(i)
		if !taken[cand] {
			return cand
		}
	}
}

func DetectFormat(name, contentType string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".csv", ".tsv":
		return "csv"
	case ".json", ".ndjson", ".jsonl":
		return "json"
	case ".parquet", ".pq":
		return "parquet"
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "csv"):
		return "csv"
	case strings.Contains(ct, "json"):
		return "json"
	case strings.Contains(ct, "parquet"):
		return "parquet"
	default:
		return ""
	}
}

func GuessSep(filename string, sample []byte) rune {
	if strings.HasSuffix(strings.ToLower(filename), ".tsv") {
		return '\t'
	}
	counts := map[rune]int{',': 0, '\t': 0, ';': 0}
	for _, b := range sample {
		r := rune(b)
		if _, ok := counts[r]; ok {
			counts[r]++
		}
	}
	best, n := ',', -1
	for r, c := range counts {
		if c > n {
			best, n = r, c
		}
	}
	return best
}
