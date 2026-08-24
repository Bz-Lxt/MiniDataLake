package exec

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"minidatalake/internal/sqlplan"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

type Stats struct {
	Scanned atomic.Int64
	OutRows atomic.Int64
}

type Result struct {
	Names  []string
	Types  []types.DataType
	Cols   []storage.Vector
	Rows   int
	Scanned int64
	Plan   []ExplainNode
}

type ExplainNode struct {
	Op     string         `json:"op"`
	Detail string         `json:"detail"`
	Rows   int64          `json:"est_rows"`
	Kids   []ExplainNode  `json:"children,omitempty"`
}

func Run(ctx context.Context, t *storage.Table, pl *sqlplan.Plan, batch int) (*Result, error) {
	if batch <= 0 {
		batch = 4096
	}
	sqlplan.ExpandStar(pl, t)
	needed := neededCols(pl, t)
	st := &Stats{}
	var rows [][]types.Value
	aliases := projectAliases(pl)

	for start := 0; start < t.Rows; start += batch {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := start + batch
		if end > t.Rows {
			end = t.Rows
		}
		vecs := t.Slice(needed, start, end)
		n := 0
		if len(vecs) > 0 {
			n = vecs[0].Len()
		}
		st.Scanned.Add(int64(n))
		env := &rowEnv{names: needed, cols: vecs}
		for i := 0; i < n; i++ {
			env.i = i
			if pl.Filter != nil && !predTrue(pl.Filter, env) {
				continue
			}
			row := evalProjects(pl, env)
			rows = append(rows, row)
			if pl.HasLimit && len(pl.Aggs) == 0 && len(pl.Groups) == 0 && !pl.Distinct && len(pl.Sorts) == 0 {
				if int64(len(rows)) >= pl.Offset+pl.Limit {
					goto done
				}
			}
		}
	}
done:
	if len(pl.Aggs) > 0 || len(pl.Groups) > 0 {
		var err error
		rows, aliases, err = aggregate(rows, aliases, pl)
		if err != nil {
			return nil, err
		}
		if pl.Having != nil {
			rows = applyHaving(rows, aliases, pl.Having)
		}
	}
	if pl.Distinct {
		rows = distinctRows(rows)
	}
	if len(pl.Sorts) > 0 {
		sortRows(rows, aliases, pl.Sorts)
	}
	if pl.HasLimit {
		rows = applyLimit(rows, pl.Offset, pl.Limit)
	}
	res := materialize(aliases, rows)
	res.Scanned = st.Scanned.Load()
	res.Plan = explainOf(pl, t, int64(res.Rows), res.Scanned)
	return res, nil
}

func neededCols(pl *sqlplan.Plan, t *storage.Table) []string {
	seen := map[string]bool{}
	var add func(*sqlplan.Expr)
	add = func(e *sqlplan.Expr) {
		if e == nil {
			return
		}
		if e.Kind == sqlplan.KCol && e.Name != "*" {
			seen[e.Name] = true
		}
		for _, k := range e.Kids {
			add(k)
		}
	}
	for _, p := range pl.Projects {
		add(p)
	}
	add(pl.Filter)
	add(pl.Having)
	for _, g := range pl.Groups {
		add(g)
	}
	for _, s := range pl.Sorts {
		add(s.Expr)
	}
	for _, a := range pl.Aggs {
		add(a.Arg)
	}
	out := make([]string, 0, len(seen))
	for _, c := range t.Cols {
		if seen[c.Meta.Name] {
			out = append(out, c.Meta.Name)
		}
	}
	if len(out) == 0 {
		for _, c := range t.Cols {
			out = append(out, c.Meta.Name)
		}
	}
	return out
}

func projectAliases(pl *sqlplan.Plan) []string {
	out := make([]string, len(pl.Projects))
	used := map[string]int{}
	for i, p := range pl.Projects {
		n := p.Alias
		if n == "" && p.Kind == sqlplan.KCol {
			n = p.Name
		}
		if n == "" && p.Kind == sqlplan.KAgg {
			n = p.Alias
			if n == "" {
				n = p.AggFn
			}
		}
		if n == "" {
			n = fmt.Sprintf("c%d", i+1)
		}
		used[n]++
		if used[n] > 1 {
			n = fmt.Sprintf("%s_%d", n, used[n])
		}
		out[i] = n
		p.Alias = n
	}
	return out
}

func evalProjects(pl *sqlplan.Plan, env *rowEnv) []types.Value {
	out := make([]types.Value, len(pl.Projects))
	for i, p := range pl.Projects {
		if p.Kind == sqlplan.KAgg {
			if p.Name == "*" || (len(p.Kids) == 1 && p.Kids[0].Kind == sqlplan.KStar) {
				out[i] = types.VInt(1)
			} else if len(p.Kids) > 0 {
				out[i] = eval(p.Kids[0], env)
			} else {
				out[i] = types.VInt(1)
			}
			continue
		}
		out[i] = eval(p, env)
	}
	return out
}

func applyLimit(rows [][]types.Value, off, lim int64) [][]types.Value {
	if off < 0 {
		off = 0
	}
	if int(off) >= len(rows) {
		return nil
	}
	rows = rows[off:]
	if lim >= 0 && int(lim) < len(rows) {
		rows = rows[:lim]
	}
	return rows
}

func distinctRows(rows [][]types.Value) [][]types.Value {
	seen := map[string]struct{}{}
	var out [][]types.Value
	for _, r := range rows {
		k := keyOf(r)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, r)
	}
	return out
}

func keyOf(r []types.Value) string {
	var b strings.Builder
	for _, v := range r {
		if v.Null {
			b.WriteString("∅|")
			continue
		}
		b.WriteString(v.String())
		b.WriteByte('|')
	}
	return b.String()
}

func materialize(names []string, rows [][]types.Value) *Result {
	ncols := len(names)
	tys := make([]types.DataType, ncols)
	for c := 0; c < ncols; c++ {
		tys[c] = types.String
		for _, r := range rows {
			if c < len(r) && !r[c].Null {
				tys[c] = r[c].Type
				break
			}
		}
	}
	if len(rows) == 0 {
		return &Result{
			Names: names, Types: tys, Cols: make([]storage.Vector, ncols), Rows: 0,
		}
	}
	builders := make([]*storage.Builder, ncols)
	for i := range builders {
		builders[i] = storage.NewBuilder(tys[i])
	}
	for _, r := range rows {
		for c := 0; c < ncols; c++ {
			if c < len(r) {
				builders[c].Append(r[c])
			} else {
				builders[c].AppendNull()
			}
		}
	}
	cols := make([]storage.Vector, ncols)
	for i, b := range builders {
		cols[i] = b.Finish()
	}
	return &Result{Names: names, Types: tys, Cols: cols, Rows: len(rows)}
}

func explainOf(pl *sqlplan.Plan, t *storage.Table, out, scanned int64) []ExplainNode {
	scan := ExplainNode{Op: "Scan", Detail: t.Name + " cols=" + strings.Join(neededCols(pl, t), ","), Rows: scanned}
	cur := scan
	if pl.Filter != nil {
		cur = ExplainNode{Op: "Filter", Detail: "WHERE", Kids: []ExplainNode{cur}, Rows: scanned}
	}
	if len(pl.Aggs) > 0 || len(pl.Groups) > 0 {
		cur = ExplainNode{Op: "HashAggregate", Detail: fmt.Sprintf("groups=%d aggs=%d", len(pl.Groups), len(pl.Aggs)), Kids: []ExplainNode{cur}, Rows: out}
	}
	if pl.Having != nil {
		cur = ExplainNode{Op: "Having", Kids: []ExplainNode{cur}, Rows: out}
	}
	if pl.Distinct {
		cur = ExplainNode{Op: "Distinct", Kids: []ExplainNode{cur}, Rows: out}
	}
	if len(pl.Sorts) > 0 {
		cur = ExplainNode{Op: "Sort", Kids: []ExplainNode{cur}, Rows: out}
	}
	if pl.HasLimit {
		cur = ExplainNode{Op: "Limit", Detail: fmt.Sprintf("limit=%d offset=%d", pl.Limit, pl.Offset), Kids: []ExplainNode{cur}, Rows: out}
	}
	return []ExplainNode{cur}
}
