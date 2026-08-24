package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"minidatalake/internal/apperr"
	"minidatalake/internal/clock"
	"minidatalake/internal/config"
	"minidatalake/internal/encoding"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

type Progress struct {
	Phase     string
	BytesDone int64
	BytesTot  int64
	Rows      int
}

type Options struct {
	Filename string
	Format   string
	Sep      rune
	Cfg      config.Config
	Log      *slog.Logger
	OnProg   func(Progress)
}

func Ingest(ctx context.Context, path string, opt Options) (*storage.Table, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	report := func(phase string, rows int, done int64) {
		if opt.OnProg != nil {
			opt.OnProg(Progress{Phase: phase, BytesDone: done, BytesTot: st.Size(), Rows: rows})
		}
	}
	report("parsing", 0, 0)

	var names []string
	var tys []types.DataType
	var vecs []storage.Vector
	var rejected []storage.RejectRow

	switch opt.Format {
	case "csv":
		sep := opt.Sep
		if sep == 0 {
			sample := make([]byte, 4096)
			n, _ := f.Read(sample)
			sep = GuessSep(opt.Filename, sample[:n])
			_, _ = f.Seek(0, 0)
		}
		chunks, err := SplitCSV(f, st.Size(), opt.Cfg.ChunkBytes, sep)
		if err != nil {
			return nil, err
		}
		head := []byte{}
		if len(chunks) > 0 {
			head = chunks[0].Data
		}
		hdr := ReadCSVHeader(head, sep)
		if len(hdr) == 0 {
			return nil, apperr.Bad("csv has no header")
		}
		names = UniqueHeaders(hdr)
		sampleRows := [][]string{}
		for _, ln := range SplitLines(head) {
			if len(sampleRows) >= 80 {
				break
			}
			sampleRows = append(sampleRows, ParseCSVLine(ln, sep))
		}
		if len(sampleRows) > 1 {
			sampleRows = sampleRows[1:]
		}
		tys = InferColumns(names, sampleRows)
		var acc int
		vecs, rejected, err = ParseCSV(ctx, chunks, names, tys, sep, true, func(rows int) {
			acc += rows
			report("parsing", acc, st.Size()/2)
		})
		if err != nil {
			return nil, err
		}
	case "json":
		names, tys, vecs, rejected, err = ParseJSON(ctx, f, opt.Cfg.MaxNestedJSON, func(rows int) {
			report("parsing", rows, st.Size()/2)
		})
		if err != nil {
			return nil, err
		}
	case "parquet":
		names, tys, vecs, err = ParseParquet(ctx, path, func(rows int) {
			report("parsing", rows, st.Size()/2)
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, apperr.New(apperr.UnsupportedFormat, 400, "unsupported format "+opt.Format)
	}
	_ = tys

	report("encoding", 0, st.Size())
	t := &storage.Table{
		SourceFile: opt.Filename, ContentHash: sum, Format: opt.Format,
		FileBytes: st.Size(), CreatedAt: clock.Format(clock.Now()), Status: "READY",
		Rejected: len(rejected), RejectSample: rejected,
	}
	if len(vecs) > 0 {
		t.Rows = vecs[0].Len()
	}
	for i, name := range names {
		ch := encoding.Choose(name, vecs[i], opt.Cfg.DictCardRatio, opt.Cfg.RLEMinRun, opt.Log)
		t.Cols = append(t.Cols, &storage.Column{Meta: ch.Meta, Vec: ch.Vec})
	}
	report("ready", t.Rows, st.Size())
	_ = bytes.TrimSpace
	_ = strings.TrimSpace
	_ = filepath.Base
	return t, nil
}
