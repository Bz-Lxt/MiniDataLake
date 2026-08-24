package sqlplan

import "minidatalake/internal/types"

type Kind int

const (
	KCol Kind = iota
	KLit
	KBin
	KUnary
	KIn
	KBetween
	KLike
	KIsNull
	KCast
	KAgg
	KStar
)

type BinOp string

const (
	OpEq  BinOp = "="
	OpNe  BinOp = "!="
	OpLt  BinOp = "<"
	OpLe  BinOp = "<="
	OpGt  BinOp = ">"
	OpGe  BinOp = ">="
	OpAnd BinOp = "AND"
	OpOr  BinOp = "OR"
	OpAdd BinOp = "+"
	OpSub BinOp = "-"
	OpMul BinOp = "*"
	OpDiv BinOp = "/"
)

type Expr struct {
	Kind     Kind
	Name     string
	Alias    string
	Lit      types.Value
	Op       BinOp
	Kids     []*Expr
	Not      bool
	AggFn    string
	Distinct bool
	CastTo   types.DataType
}

func Col(name string) *Expr { return &Expr{Kind: KCol, Name: name} }
func Lit(v types.Value) *Expr {
	return &Expr{Kind: KLit, Lit: v}
}
func Bin(op BinOp, a, b *Expr) *Expr {
	return &Expr{Kind: KBin, Op: op, Kids: []*Expr{a, b}}
}

func (e *Expr) Columns() []string {
	if e == nil {
		return nil
	}
	switch e.Kind {
	case KCol:
		return []string{e.Name}
	case KAgg:
		var out []string
		for _, k := range e.Kids {
			out = append(out, k.Columns()...)
		}
		if e.Name != "" && e.Name != "*" {
			out = append(out, e.Name)
		}
		return out
	default:
		var out []string
		for _, k := range e.Kids {
			out = append(out, k.Columns()...)
		}
		return out
	}
}

func (e *Expr) IsAgg() bool {
	if e == nil {
		return false
	}
	if e.Kind == KAgg {
		return true
	}
	for _, k := range e.Kids {
		if k.IsAgg() {
			return true
		}
	}
	return false
}

type AggSpec struct {
	Fn       string
	Arg      *Expr
	Distinct bool
	Alias    string
}

type SortKey struct {
	Expr *Expr
	Desc bool
}

type Plan struct {
	Table    string
	Alias    string
	Projects []*Expr
	Filter   *Expr
	Groups   []*Expr
	Aggs     []AggSpec
	Having   *Expr
	Sorts    []SortKey
	Limit    int64
	Offset   int64
	HasLimit bool
	Distinct bool
	Explain  bool
	RawSQL   string
}
