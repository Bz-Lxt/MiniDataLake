package ingest

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/parquet-go/parquet-go"

	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func ParseParquet(ctx context.Context, path string, onProg func(rows int)) ([]string, []types.DataType, []storage.Vector, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, nil, err
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		return nil, nil, nil, err
	}
	schema := pf.Schema()
	leaves := schema.Fields()
	var names []string
	var tys []types.DataType
	var idx []int
	for i, field := range leaves {
		if !field.Leaf() {
			continue
		}
		idx = append(idx, i)
		names = append(names, types.Ident(field.Name()))
		tys = append(tys, mapParquetType(field.Type().Kind()))
	}
	if len(names) == 0 {
		return nil, nil, nil, fmt.Errorf("parquet has no leaf columns")
	}
	builders := make([]*storage.Builder, len(names))
	for i := range builders {
		builders[i] = storage.NewBuilder(tys[i])
	}

	rd := parquet.NewReader(pf)
	defer rd.Close()
	buf := make([]parquet.Row, 256)
	read := 0
	for {
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		default:
		}
		n, err := rd.ReadRows(buf)
		for i := 0; i < n; i++ {
			row := buf[i]
			byCol := map[int]parquet.Value{}
			for _, v := range row {
				byCol[v.Column()] = v
			}
			for c := range names {
				pv, ok := byCol[idx[c]]
				if !ok || pv.IsNull() {
					builders[c].AppendNull()
					continue
				}
				builders[c].Append(parquetVal(pv, tys[c]))
			}
		}
		read += n
		if onProg != nil {
			onProg(n)
		}
		if n == 0 || err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, nil, err
		}
	}
	vecs := make([]storage.Vector, len(builders))
	for i, b := range builders {
		vecs[i] = b.Finish()
	}
	return names, tys, vecs, nil
}

func mapParquetType(k parquet.Kind) types.DataType {
	switch k {
	case parquet.Int32, parquet.Int64, parquet.Int96:
		return types.Int64
	case parquet.Float, parquet.Double:
		return types.Float64
	case parquet.Boolean:
		return types.Bool
	default:
		return types.String
	}
}

func parquetVal(v parquet.Value, t types.DataType) types.Value {
	switch t {
	case types.Int64:
		return types.VInt(v.Int64())
	case types.Float64:
		return types.VFloat(v.Double())
	case types.Bool:
		return types.VBool(v.Boolean())
	default:
		if s := v.String(); s != "" {
			return types.VStr(s)
		}
		return types.VStr(fmt.Sprint(v))
	}
}
