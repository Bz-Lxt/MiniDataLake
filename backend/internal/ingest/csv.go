package ingest

import (
	"bytes"
	"context"
	"runtime"
	"sync"

	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

type csvResult struct {
	idx      int
	cols     [][]types.Value
	rejected []storage.RejectRow
	err      error
}

func ParseCSV(ctx context.Context, chunks []Chunk, headers []string, tys []types.DataType, sep rune, skipHeader bool, onProg func(rows int)) ([]storage.Vector, []storage.RejectRow, error) {
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		workers = 2
	}
	jobs := make(chan Chunk, len(chunks))
	out := make(chan csvResult, len(chunks))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ch := range jobs {
				select {
				case <-ctx.Done():
					out <- csvResult{idx: ch.Index, err: ctx.Err()}
					return
				default:
				}
				res := parseCSVChunk(ch, headers, tys, sep, skipHeader && ch.Index == 0)
				out <- res
			}
		}()
	}
	for _, c := range chunks {
		jobs <- c
	}
	close(jobs)
	go func() { wg.Wait(); close(out) }()

	byIdx := make([]csvResult, len(chunks))
	got := 0
	var firstErr error
	for r := range out {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		if r.idx >= 0 && r.idx < len(byIdx) {
			byIdx[r.idx] = r
		}
		got++
		if onProg != nil {
			rows := 0
			if r.cols != nil && len(r.cols) > 0 {
				rows = len(r.cols[0])
			}
			onProg(rows)
		}
	}
	if firstErr != nil {
		return nil, nil, firstErr
	}

	builders := make([]*storage.Builder, len(headers))
	for i := range builders {
		builders[i] = storage.NewBuilder(tys[i])
	}
	var rejected []storage.RejectRow
	for _, r := range byIdx {
		if r.cols == nil {
			continue
		}
		n := 0
		if len(r.cols) > 0 {
			n = len(r.cols[0])
		}
		for row := 0; row < n; row++ {
			for c := range builders {
				if c < len(r.cols) && row < len(r.cols[c]) {
					builders[c].Append(r.cols[c][row])
				} else {
					builders[c].AppendNull()
				}
			}
		}
		rejected = append(rejected, r.rejected...)
		if len(rejected) > 50 {
			rejected = rejected[:50]
		}
	}
	vecs := make([]storage.Vector, len(builders))
	for i, b := range builders {
		vecs[i] = b.Finish()
	}
	return vecs, rejected, nil
}

func parseCSVChunk(ch Chunk, headers []string, tys []types.DataType, sep rune, skipHeader bool) csvResult {
	lines := SplitLines(ch.Data)
	if skipHeader && len(lines) > 0 {
		lines = lines[1:]
	}
	ncols := len(headers)
	cols := make([][]types.Value, ncols)
	var rejected []storage.RejectRow
	lineNo := int(ch.Start)
	for _, line := range lines {
		lineNo++
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		fields := ParseCSVLine(line, sep)
		if len(fields) != ncols {
			if len(rejected) < 20 {
				sample := string(line)
				if len(sample) > 120 {
					sample = sample[:120]
				}
				rejected = append(rejected, storage.RejectRow{Line: lineNo, Reason: "column count mismatch", Sample: sample})
			}
			if len(fields) < ncols {
				for len(fields) < ncols {
					fields = append(fields, "")
				}
			} else {
				fields = fields[:ncols]
			}
		}
		for c := 0; c < ncols; c++ {
			v, ok := types.ParseCell(fields[c], tys[c])
			if !ok {
				v = types.Null(tys[c])
			}
			cols[c] = append(cols[c], v)
		}
	}
	return csvResult{idx: ch.Index, cols: cols, rejected: rejected}
}

func ReadCSVHeader(data []byte, sep rune) []string {
	lines := SplitLines(data)
	if len(lines) == 0 {
		return nil
	}
	return ParseCSVLine(lines[0], sep)
}
