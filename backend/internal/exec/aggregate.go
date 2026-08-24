package exec

import (
	"strings"
	"sync"

	"minidatalake/internal/sqlplan"
	"minidatalake/internal/types"
)

type acc struct {
	count    int64
	sum      float64
	min      types.Value
	max      types.Value
	has      bool
	distinct map[string]struct{}
}

type slot struct {
	key  []types.Value
	accs []acc
}

func aggregate(rows [][]types.Value, aliases []string, pl *sqlplan.Plan) ([][]types.Value, []string, error) {
	gidx := groupIndexes(pl, aliases)
	aidx := aggIndexes(pl)
	m := map[string]*slot{}
	var mu sync.Mutex
	for _, row := range rows {
		key := make([]types.Value, len(gidx))
		for i, gi := range gidx {
			if gi >= 0 && gi < len(row) {
				key[i] = row[gi]
			}
		}
		ks := keyOf(key)
		mu.Lock()
		s, ok := m[ks]
		if !ok {
			s = &slot{key: key, accs: make([]acc, len(pl.Aggs))}
			m[ks] = s
		}
		for i, spec := range pl.Aggs {
			updateAcc(&s.accs[i], spec, row, aliases)
		}
		mu.Unlock()
	}

	outNames := make([]string, 0, len(gidx)+len(pl.Aggs))
	for _, g := range pl.Groups {
		n := g.Alias
		if n == "" && g.Kind == sqlplan.KCol {
			n = g.Name
		}
		if n == "" {
			n = "grp"
		}
		outNames = append(outNames, n)
	}
	for _, a := range pl.Aggs {
		outNames = append(outNames, a.Alias)
	}
	if len(outNames) == 0 {
		outNames = aliases
	}

	var out [][]types.Value
	if len(m) == 0 && len(pl.Groups) == 0 {
		row := finishSlot(&slot{accs: make([]acc, len(pl.Aggs))}, pl, nil)
		out = append(out, row)
		return out, outNames, nil
	}
	for _, s := range m {
		out = append(out, finishSlot(s, pl, outNames))
	}
	_ = aidx
	return out, outNames, nil
}

func groupIndexes(pl *sqlplan.Plan, aliases []string) []int {
	var idx []int
	for _, g := range pl.Groups {
		name := g.Alias
		if name == "" && g.Kind == sqlplan.KCol {
			name = g.Name
		}
		found := -1
		for i, a := range aliases {
			if strings.EqualFold(a, name) || (g.Kind == sqlplan.KCol && strings.EqualFold(aliases[i], g.Name)) {
				found = i
				break
			}
			if pl.Projects[i].Kind == sqlplan.KCol && strings.EqualFold(pl.Projects[i].Name, g.Name) {
				found = i
				break
			}
		}
		idx = append(idx, found)
	}
	return idx
}

func aggIndexes(pl *sqlplan.Plan) []int {
	var idx []int
	for i, p := range pl.Projects {
		if p.Kind == sqlplan.KAgg {
			idx = append(idx, i)
		}
	}
	return idx
}

func updateAcc(a *acc, spec sqlplan.AggSpec, row []types.Value, aliases []string) {
	var v types.Value
	if spec.Arg == nil || spec.Arg.Kind == sqlplan.KStar || spec.Fn == "COUNT" && spec.Arg != nil && spec.Arg.Name == "*" {
		if spec.Fn == "COUNT" && (spec.Arg == nil || spec.Arg.Kind == sqlplan.KStar || spec.Arg.Name == "*") {
			a.count++
			return
		}
	}
	v = argValue(spec, row, aliases)
	if spec.Fn == "COUNT" {
		if spec.Arg == nil || spec.Arg.Kind == sqlplan.KStar {
			a.count++
			return
		}
		if !v.Null {
			if spec.Distinct {
				if a.distinct == nil {
					a.distinct = map[string]struct{}{}
				}
				a.distinct[v.String()] = struct{}{}
			} else {
				a.count++
			}
		}
		return
	}
	if v.Null {
		return
	}
	if spec.Distinct {
		if a.distinct == nil {
			a.distinct = map[string]struct{}{}
		}
		k := v.String()
		if _, ok := a.distinct[k]; ok {
			return
		}
		a.distinct[k] = struct{}{}
	}
	switch spec.Fn {
	case "SUM", "AVG":
		f, ok := v.AsFloat()
		if ok {
			a.sum += f
			a.count++
		}
	case "MIN":
		if !a.has || types.Compare(v, a.min) < 0 {
			a.min = v
			a.has = true
		}
	case "MAX":
		if !a.has || types.Compare(v, a.max) > 0 {
			a.max = v
			a.has = true
		}
	}
}

func argValue(spec sqlplan.AggSpec, row []types.Value, aliases []string) types.Value {
	if spec.Arg == nil {
		return types.VInt(1)
	}
	if spec.Arg.Kind == sqlplan.KCol {
		for i, a := range aliases {
			if strings.EqualFold(a, spec.Arg.Name) || strings.EqualFold(a, spec.Alias) {
				if i < len(row) {
					return row[i]
				}
			}
		}
		// project list stores raw agg arg values in the agg project slot
		for i := range aliases {
			if i < len(row) && strings.Contains(strings.ToUpper(aliases[i]), spec.Fn) {
				return row[i]
			}
		}
	}
	for i := range aliases {
		if strings.EqualFold(aliases[i], spec.Alias) && i < len(row) {
			return row[i]
		}
	}
	return types.Null(types.String)
}

func finishSlot(s *slot, pl *sqlplan.Plan, _ []string) []types.Value {
	row := make([]types.Value, 0, len(s.key)+len(pl.Aggs))
	row = append(row, s.key...)
	for i, spec := range pl.Aggs {
		a := s.accs[i]
		switch spec.Fn {
		case "COUNT":
			if spec.Distinct && a.distinct != nil {
				row = append(row, types.VInt(int64(len(a.distinct))))
			} else {
				row = append(row, types.VInt(a.count))
			}
		case "SUM":
			row = append(row, types.VFloat(a.sum))
		case "AVG":
			if a.count == 0 {
				row = append(row, types.Null(types.Float64))
			} else {
				row = append(row, types.VFloat(a.sum/float64(a.count)))
			}
		case "MIN":
			if !a.has {
				row = append(row, types.Null(types.String))
			} else {
				row = append(row, a.min)
			}
		case "MAX":
			if !a.has {
				row = append(row, types.Null(types.String))
			} else {
				row = append(row, a.max)
			}
		default:
			row = append(row, types.Null(types.String))
		}
	}
	return row
}

func applyHaving(rows [][]types.Value, names []string, having *sqlplan.Expr) [][]types.Value {
	var out [][]types.Value
	env := &rowEnv{alias: map[string]types.Value{}}
	for i := range rows {
		for j, n := range names {
			if j < len(rows[i]) {
				env.alias[strings.ToLower(n)] = rows[i][j]
			}
		}
		if predTrue(having, env) {
			out = append(out, rows[i])
		}
	}
	return out
}
