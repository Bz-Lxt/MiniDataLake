package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func ParseJSON(ctx context.Context, r io.Reader, maxDepth int, onProg func(rows int)) ([]string, []types.DataType, []storage.Vector, []storage.RejectRow, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("empty json")
	}

	var rows []map[string]string
	var rejected []storage.RejectRow
	if raw[0] == '[' {
		var arr []any
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, nil, nil, nil, err
		}
		for i, item := range arr {
			select {
			case <-ctx.Done():
				return nil, nil, nil, nil, ctx.Err()
			default:
			}
			m, ok := item.(map[string]any)
			if !ok {
				rejected = append(rejected, storage.RejectRow{Line: i + 1, Reason: "array element is not object"})
				continue
			}
			rows = append(rows, flatten(m, "", maxDepth, 0))
			if onProg != nil && (i+1)%1024 == 0 {
				onProg(1)
			}
		}
	} else {
		dec := json.NewDecoder(bytes.NewReader(raw))
		line := 0
		for {
			var item any
			if err := dec.Decode(&item); err != nil {
				if err == io.EOF {
					break
				}
				rejected = append(rejected, storage.RejectRow{Line: line + 1, Reason: err.Error()})
				continue
			}
			line++
			m, ok := item.(map[string]any)
			if !ok {
				rejected = append(rejected, storage.RejectRow{Line: line, Reason: "line is not object"})
				continue
			}
			rows = append(rows, flatten(m, "", maxDepth, 0))
			if onProg != nil && line%1024 == 0 {
				onProg(1)
			}
		}
	}

	keyset := map[string]struct{}{}
	for _, row := range rows {
		for k := range row {
			keyset[k] = struct{}{}
		}
	}
	headers := make([]string, 0, len(keyset))
	for k := range keyset {
		headers = append(headers, k)
	}
	sort.Strings(headers)
	headers = UniqueHeaders(headers)

	samples := make([][]string, 0, min(len(rows), 200))
	for i := 0; i < len(rows) && i < 200; i++ {
		line := make([]string, len(headers))
		for c, h := range headers {
			line[c] = rows[i][h]
		}
		samples = append(samples, line)
	}
	tys := InferColumns(headers, samples)

	builders := make([]*storage.Builder, len(headers))
	for i := range builders {
		builders[i] = storage.NewBuilder(tys[i])
	}
	for _, row := range rows {
		for c, h := range headers {
			v, ok := types.ParseCell(row[h], tys[c])
			if !ok {
				v = types.Null(tys[c])
			}
			if row[h] == "" {
				builders[c].AppendNull()
			} else {
				builders[c].Append(v)
			}
		}
	}
	vecs := make([]storage.Vector, len(builders))
	for i, b := range builders {
		vecs[i] = b.Finish()
	}
	if len(rejected) > 50 {
		rejected = rejected[:50]
	}
	return headers, tys, vecs, rejected, nil
}

func flatten(m map[string]any, prefix string, maxDepth, depth int) map[string]string {
	out := map[string]string{}
	if depth > maxDepth {
		b, _ := json.Marshal(m)
		out[prefix] = string(b)
		return out
	}
	for k, v := range m {
		name := types.Ident(k)
		if prefix != "" {
			name = types.Ident(prefix + "_" + k)
		}
		switch t := v.(type) {
		case nil:
			out[name] = ""
		case map[string]any:
			for kk, vv := range flatten(t, name, maxDepth, depth+1) {
				out[kk] = vv
			}
		case []any:
			b, _ := json.Marshal(t)
			out[name] = string(b)
		case float64:
			if t == float64(int64(t)) {
				out[name] = strconv.FormatInt(int64(t), 10)
			} else {
				out[name] = strconv.FormatFloat(t, 'f', -1, 64)
			}
		case bool:
			out[name] = strconv.FormatBool(t)
		case string:
			out[name] = t
		default:
			out[name] = fmt.Sprint(t)
		}
	}
	_ = strings.TrimSpace
	return out
}
