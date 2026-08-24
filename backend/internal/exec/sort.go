package exec

import (
	"sort"
	"strings"

	"minidatalake/internal/sqlplan"
	"minidatalake/internal/types"
)

func sortRows(rows [][]types.Value, names []string, keys []sqlplan.SortKey) {
	idx := make([]int, len(keys))
	for i, k := range keys {
		idx[i] = -1
		n := ""
		if k.Expr != nil {
			n = k.Expr.Alias
			if n == "" && k.Expr.Kind == sqlplan.KCol {
				n = k.Expr.Name
			}
		}
		for j, nm := range names {
			if strings.EqualFold(nm, n) {
				idx[i] = j
				break
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		for k, key := range keys {
			ci := idx[k]
			if ci < 0 {
				continue
			}
			a, b := types.Null(types.String), types.Null(types.String)
			if ci < len(rows[i]) {
				a = rows[i][ci]
			}
			if ci < len(rows[j]) {
				b = rows[j][ci]
			}
			c := types.Compare(a, b)
			if c == 0 {
				continue
			}
			if key.Desc {
				return c > 0
			}
			return c < 0
		}
		return false
	})
}
