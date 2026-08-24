package encoding

import (
	"log/slog"

	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

type Choice struct {
	Vec    storage.Vector
	Meta   storage.ColumnMeta
}

func Choose(name string, src storage.Vector, dictRatio, rleMin float64, log *slog.Logger) Choice {
	meta := storage.ColumnMeta{
		Name:     name,
		Type:     src.Type(),
		Encoding: types.Plain,
		Rows:     src.Len(),
		Nulls:    src.NullCount(),
		RawBytes: src.MemBytes(),
		EncBytes: src.MemBytes(),
		Reason:   "plain: no profitable encoding",
	}
	n := src.Len()
	if n == 0 {
		return Choice{Vec: src, Meta: meta}
	}

	if src.Type() == types.String {
		if sv, ok := src.(*storage.StrVec); ok {
			card := cardinality(sv)
			meta.Card = card
			ratio := float64(card) / float64(n)
			if ratio <= dictRatio {
				dv := EncodeDict(sv)
				meta.Encoding = types.Dict
				meta.EncBytes = dv.MemBytes()
				meta.Reason = "dict: cardinality/rows below threshold"
				if log != nil {
					log.Info("encoding chosen", "col", name, "enc", "DICT", "card", card, "rows", n, "ratio", ratio)
				}
				return Choice{Vec: dv, Meta: meta}
			}
		}
	}

	avg := AvgRun(src)
	meta.AvgRun = avg
	if avg >= rleMin {
		rv := EncodeRLE(src)
		meta.Encoding = types.RLE
		meta.EncBytes = rv.MemBytes()
		meta.Reason = "rle: average run length above threshold"
		if log != nil {
			log.Info("encoding chosen", "col", name, "enc", "RLE", "avg_run", avg, "rows", n)
		}
		return Choice{Vec: rv, Meta: meta}
	}

	if log != nil {
		log.Info("encoding chosen", "col", name, "enc", "PLAIN", "avg_run", avg, "card", meta.Card)
	}
	return Choice{Vec: src, Meta: meta}
}

func cardinality(sv *storage.StrVec) int {
	seen := map[string]struct{}{}
	for i := 0; i < sv.Len(); i++ {
		if sv.IsNull(i) {
			continue
		}
		seen[sv.At(i)] = struct{}{}
	}
	return len(seen)
}
