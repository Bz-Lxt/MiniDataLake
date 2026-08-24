package exec

import (
	"math"
	"regexp"
	"strings"

	"minidatalake/internal/sqlplan"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

type rowEnv struct {
	names []string
	cols  []storage.Vector
	i     int
	alias map[string]types.Value
}

func (e *rowEnv) get(name string) types.Value {
	if e.alias != nil {
		if v, ok := e.alias[strings.ToLower(name)]; ok {
			return v
		}
	}
	for i, n := range e.names {
		if strings.EqualFold(n, name) {
			return e.cols[i].Get(e.i)
		}
	}
	return types.Null(types.String)
}

func eval(e *sqlplan.Expr, env *rowEnv) types.Value {
	if e == nil {
		return types.Null(types.String)
	}
	switch e.Kind {
	case sqlplan.KCol:
		return env.get(e.Name)
	case sqlplan.KLit:
		return e.Lit
	case sqlplan.KCast:
		v := eval(e.Kids[0], env)
		return cast(v, e.CastTo)
	case sqlplan.KUnary:
		v := eval(e.Kids[0], env)
		if e.Op == "NOT" {
			if v.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(!truth(v))
		}
		return v
	case sqlplan.KIsNull:
		v := eval(e.Kids[0], env)
		ok := v.Null
		if e.Not {
			ok = !ok
		}
		return types.VBool(ok)
	case sqlplan.KLike:
		a := eval(e.Kids[0], env)
		p := eval(e.Kids[1], env)
		if a.Null || p.Null {
			return types.Null(types.Bool)
		}
		ok := matchLike(a.String(), p.String())
		if e.Not {
			ok = !ok
		}
		return types.VBool(ok)
	case sqlplan.KIn:
		a := eval(e.Kids[0], env)
		if a.Null {
			return types.Null(types.Bool)
		}
		ok := false
		for _, k := range e.Kids[1:] {
			if types.Compare(a, eval(k, env)) == 0 {
				ok = true
				break
			}
		}
		if e.Not {
			ok = !ok
		}
		return types.VBool(ok)
	case sqlplan.KBetween:
		a := eval(e.Kids[0], env)
		lo := eval(e.Kids[1], env)
		hi := eval(e.Kids[2], env)
		if a.Null || lo.Null || hi.Null {
			return types.Null(types.Bool)
		}
		ok := types.Compare(a, lo) >= 0 && types.Compare(a, hi) <= 0
		if e.Not {
			ok = !ok
		}
		return types.VBool(ok)
	case sqlplan.KBin:
		l := eval(e.Kids[0], env)
		if e.Op == sqlplan.OpAnd {
			if l.Null {
				r := eval(e.Kids[1], env)
				if r.Null || !truth(r) {
					if r.Null {
						return types.Null(types.Bool)
					}
					return types.VBool(false)
				}
				return types.Null(types.Bool)
			}
			if !truth(l) {
				return types.VBool(false)
			}
			return eval(e.Kids[1], env)
		}
		if e.Op == sqlplan.OpOr {
			if !l.Null && truth(l) {
				return types.VBool(true)
			}
			r := eval(e.Kids[1], env)
			if !r.Null && truth(r) {
				return types.VBool(true)
			}
			if l.Null || r.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(false)
		}
		r := eval(e.Kids[1], env)
		switch e.Op {
		case sqlplan.OpEq:
			if l.Null || r.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(types.Compare(l, r) == 0)
		case sqlplan.OpNe:
			if l.Null || r.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(types.Compare(l, r) != 0)
		case sqlplan.OpLt:
			if l.Null || r.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(types.Compare(l, r) < 0)
		case sqlplan.OpLe:
			if l.Null || r.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(types.Compare(l, r) <= 0)
		case sqlplan.OpGt:
			if l.Null || r.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(types.Compare(l, r) > 0)
		case sqlplan.OpGe:
			if l.Null || r.Null {
				return types.Null(types.Bool)
			}
			return types.VBool(types.Compare(l, r) >= 0)
		case sqlplan.OpAdd, sqlplan.OpSub, sqlplan.OpMul, sqlplan.OpDiv:
			return arith(e.Op, l, r)
		}
	case sqlplan.KAgg:
		if e.Alias != "" {
			return env.get(e.Alias)
		}
	}
	return types.Null(types.String)
}

func truth(v types.Value) bool {
	if v.Null {
		return false
	}
	switch v.Type {
	case types.Bool:
		return v.B
	case types.Int64:
		return v.I != 0
	case types.Float64:
		return v.F != 0
	default:
		return v.S != ""
	}
}

func arith(op sqlplan.BinOp, l, r types.Value) types.Value {
	if l.Null || r.Null {
		return types.Null(types.Float64)
	}
	lf, ok1 := l.AsFloat()
	rf, ok2 := r.AsFloat()
	if !ok1 || !ok2 {
		return types.Null(types.Float64)
	}
	switch op {
	case sqlplan.OpAdd:
		return types.VFloat(lf + rf)
	case sqlplan.OpSub:
		return types.VFloat(lf - rf)
	case sqlplan.OpMul:
		return types.VFloat(lf * rf)
	case sqlplan.OpDiv:
		if rf == 0 {
			return types.Null(types.Float64)
		}
		return types.VFloat(lf / rf)
	}
	return types.Null(types.Float64)
}

func cast(v types.Value, t types.DataType) types.Value {
	if v.Null {
		return types.Null(t)
	}
	nv, ok := types.ParseCell(v.String(), t)
	if !ok {
		return types.Null(t)
	}
	return nv
}

func matchLike(s, pat string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch c {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		case '.', '(', ')', '+', '*', '?', '[', ']', '{', '}', '^', '$', '|', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile("(?is)" + b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

func predTrue(e *sqlplan.Expr, env *rowEnv) bool {
	v := eval(e, env)
	if v.Null {
		return false
	}
	return truth(v)
}

func isFinite(v types.Value) bool {
	if v.Type != types.Float64 {
		return true
	}
	return !math.IsNaN(v.F) && !math.IsInf(v.F, 0)
}
