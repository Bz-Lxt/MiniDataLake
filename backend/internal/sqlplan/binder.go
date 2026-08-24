package sqlplan

import (
	"strings"

	"minidatalake/internal/apperr"
	"minidatalake/internal/storage"
)

func Bind(pl *Plan, t *storage.Table) error {
	if t == nil {
		return apperr.Miss("table not found: " + pl.Table)
	}
	if t.Status == "CORRUPTED" {
		return apperr.New(apperr.TableCorrupted, 409, "table is corrupted: "+t.Name)
	}
	names := map[string]bool{}
	for _, c := range t.Cols {
		names[strings.ToLower(c.Meta.Name)] = true
	}
	resolve := func(n string) (string, error) {
		if n == "*" {
			return n, nil
		}
		low := strings.ToLower(n)
		if names[low] {
			for _, c := range t.Cols {
				if strings.EqualFold(c.Meta.Name, n) {
					return c.Meta.Name, nil
				}
			}
		}
		sug := suggest(n, t)
		msg := "unknown column " + n
		if sug != "" {
			msg += ", did you mean " + sug + "?"
		}
		return "", apperr.Sem(msg).With("column", n)
	}
	var walk func(*Expr) error
	walk = func(e *Expr) error {
		if e == nil {
			return nil
		}
		if e.Kind == KCol {
			real, err := resolve(e.Name)
			if err != nil {
				return err
			}
			e.Name = real
		}
		for _, k := range e.Kids {
			if err := walk(k); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(pl.Filter); err != nil {
		return err
	}
	aliases := map[string]bool{}
	for _, p := range pl.Projects {
		if p.Alias != "" {
			aliases[strings.ToLower(p.Alias)] = true
		}
	}
	var walkAlias func(*Expr) error
	walkAlias = func(e *Expr) error {
		if e == nil {
			return nil
		}
		if e.Kind == KCol {
			if aliases[strings.ToLower(e.Name)] {
				return nil
			}
			real, err := resolve(e.Name)
			if err != nil {
				return err
			}
			e.Name = real
		}
		for _, k := range e.Kids {
			if err := walkAlias(k); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walkAlias(pl.Having); err != nil {
		return err
	}
	for _, g := range pl.Groups {
		if err := walk(g); err != nil {
			return err
		}
	}
	for i := range pl.Sorts {
		if err := walkAlias(pl.Sorts[i].Expr); err != nil {
			return err
		}
	}
	hasStar := false
	for _, p := range pl.Projects {
		if p.Kind == KStar {
			hasStar = true
			continue
		}
		if err := walk(p); err != nil {
			return err
		}
	}
	if hasStar && len(pl.Projects) > 1 {
		// keep * with extras — expand later
	}
	if len(pl.Groups) > 0 || len(pl.Aggs) > 0 {
		for _, p := range pl.Projects {
			if p.Kind == KStar {
				continue
			}
			if p.IsAgg() {
				continue
			}
			if !inGroup(p, pl.Groups) {
				return apperr.Sem("column " + display(p) + " must appear in GROUP BY or an aggregate")
			}
		}
	}
	return nil
}

func inGroup(e *Expr, groups []*Expr) bool {
	if e == nil {
		return false
	}
	for _, g := range groups {
		if sameCol(e, g) {
			return true
		}
	}
	return false
}

func sameCol(a, b *Expr) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Kind == KCol && b.Kind == KCol {
		return strings.EqualFold(a.Name, b.Name)
	}
	return false
}

func display(e *Expr) string {
	if e == nil {
		return "?"
	}
	if e.Alias != "" {
		return e.Alias
	}
	if e.Kind == KCol {
		return e.Name
	}
	return e.AggFn
}

func suggest(name string, t *storage.Table) string {
	low := strings.ToLower(name)
	best := ""
	bestD := 99
	for _, c := range t.Cols {
		d := levenshtein(low, strings.ToLower(c.Meta.Name))
		if d < bestD && d <= 2 {
			bestD = d
			best = c.Meta.Name
		}
	}
	return best
}

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	dp := make([]int, len(b)+1)
	for j := range dp {
		dp[j] = j
	}
	for i := 1; i <= len(a); i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= len(b); j++ {
			cur := dp[j]
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			ins := dp[j] + 1
			del := dp[j-1] + 1
			sub := prev + cost
			m := ins
			if del < m {
				m = del
			}
			if sub < m {
				m = sub
			}
			dp[j] = m
			prev = cur
		}
	}
	return dp[len(b)]
}

func ExpandStar(pl *Plan, t *storage.Table) {
	out := make([]*Expr, 0, len(pl.Projects)+len(t.Cols))
	for _, p := range pl.Projects {
		if p.Kind == KStar {
			for _, c := range t.Cols {
				out = append(out, Col(c.Meta.Name))
			}
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		for _, c := range t.Cols {
			out = append(out, Col(c.Meta.Name))
		}
	}
	pl.Projects = out
}
