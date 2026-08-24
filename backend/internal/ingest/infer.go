package ingest

import "minidatalake/internal/types"

func InferColumns(headers []string, samples [][]string) []types.DataType {
	n := len(headers)
	out := make([]types.DataType, n)
	votes := make([]map[types.DataType]int, n)
	for i := range votes {
		votes[i] = map[types.DataType]int{}
	}
	for _, row := range samples {
		for i := 0; i < n && i < len(row); i++ {
			t := types.InferSample(row[i])
			if t != types.Invalid {
				votes[i][t]++
			}
		}
	}
	for i := 0; i < n; i++ {
		out[i] = pick(votes[i])
	}
	return out
}

func pick(v map[types.DataType]int) types.DataType {
	if len(v) == 0 {
		return types.String
	}
	if v[types.String] > 0 && mixed(v) {
		return types.String
	}
	order := []types.DataType{types.Bool, types.Int64, types.Float64, types.Timestamp, types.String}
	best := types.String
	bestN := -1
	for _, t := range order {
		if v[t] > bestN {
			best, bestN = t, v[t]
		}
	}
	if v[types.Float64] > 0 && v[types.Int64] > 0 {
		return types.Float64
	}
	return best
}

func mixed(v map[types.DataType]int) bool {
	kinds := 0
	for t, n := range v {
		if n > 0 && t != types.Invalid {
			kinds++
		}
	}
	return kinds > 1 && v[types.String] > 0
}

func UniqueHeaders(in []string) []string {
	seen := map[string]int{}
	out := make([]string, len(in))
	for i, h := range in {
		name := types.Ident(h)
		if name == "" {
			name = "col"
		}
		seen[name]++
		if seen[name] > 1 {
			name = name + "_" + itoa(seen[name])
		}
		out[i] = name
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
