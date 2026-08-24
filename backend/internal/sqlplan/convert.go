package sqlplan

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/opcode"
	"github.com/pingcap/tidb/pkg/parser/test_driver"

	"minidatalake/internal/apperr"
	"minidatalake/internal/types"
)

func Parse(sql string) (*Plan, error) {
	sql = strings.TrimSpace(sql)
	explain := false
	up := strings.ToUpper(sql)
	if strings.HasPrefix(up, "EXPLAIN ") {
		explain = true
		sql = strings.TrimSpace(sql[8:])
	}
	p := parser.New()
	stmts, _, err := p.Parse(sql, "", "")
	if err != nil {
		return nil, apperr.Bad("SQL parse error: " + err.Error())
	}
	if len(stmts) != 1 {
		return nil, apperr.Bad("exactly one statement is allowed")
	}
	switch s := stmts[0].(type) {
	case *ast.SelectStmt:
		pl, err := fromSelect(s)
		if err != nil {
			return nil, err
		}
		pl.Explain = explain
		pl.RawSQL = sql
		return pl, nil
	case *ast.InsertStmt:
		return nil, apperr.Unsup("INSERT", "this workspace is read-only; ingest files instead")
	case *ast.UpdateStmt:
		return nil, apperr.Unsup("UPDATE", "this workspace is read-only")
	case *ast.DeleteStmt:
		return nil, apperr.Unsup("DELETE", "this workspace is read-only")
	case *ast.CreateTableStmt, *ast.DropTableStmt, *ast.AlterTableStmt:
		return nil, apperr.Unsup("DDL", "tables are created by file ingest")
	default:
		return nil, apperr.Unsup(fmt.Sprintf("%T", stmts[0]), "only SELECT / EXPLAIN SELECT is supported")
	}
}

func fromSelect(s *ast.SelectStmt) (*Plan, error) {
	if s.With != nil {
		return nil, apperr.Unsup("CTE (WITH)", "rewrite as a single SELECT")
	}
	if s.From == nil || s.From.TableRefs == nil {
		return nil, apperr.Bad("FROM clause is required")
	}
	if err := rejectJoin(s.From); err != nil {
		return nil, err
	}
	tn, alias, err := tableName(s.From.TableRefs)
	if err != nil {
		return nil, err
	}
	pl := &Plan{Table: tn, Alias: alias}
	if s.Distinct {
		pl.Distinct = true
	}
	if s.Where != nil {
		e, err := convExpr(s.Where)
		if err != nil {
			return nil, err
		}
		pl.Filter = e
	}
	if s.Fields != nil {
		for _, f := range s.Fields.Fields {
			e, err := convField(f)
			if err != nil {
				return nil, err
			}
			pl.Projects = append(pl.Projects, e)
		}
	}
	if s.GroupBy != nil {
		for _, it := range s.GroupBy.Items {
			e, err := convExpr(it.Expr)
			if err != nil {
				return nil, err
			}
			pl.Groups = append(pl.Groups, e)
		}
	}
	if s.Having != nil {
		e, err := convExpr(s.Having.Expr)
		if err != nil {
			return nil, err
		}
		pl.Having = e
	}
	if s.OrderBy != nil {
		for _, it := range s.OrderBy.Items {
			e, err := convExpr(it.Expr)
			if err != nil {
				return nil, err
			}
			pl.Sorts = append(pl.Sorts, SortKey{Expr: e, Desc: it.Desc})
		}
	}
	if s.Limit != nil {
		pl.HasLimit = true
		if s.Limit.Count != nil {
			n, err := constInt(s.Limit.Count)
			if err != nil {
				return nil, err
			}
			pl.Limit = n
		}
		if s.Limit.Offset != nil {
			n, err := constInt(s.Limit.Offset)
			if err != nil {
				return nil, err
			}
			pl.Offset = n
		}
	}
	if s.WindowSpecs != nil || hasWindow(s) {
		return nil, apperr.Unsup("window function (OVER)", "use GROUP BY aggregates instead")
	}
	collectAggs(pl)
	return pl, nil
}

func rejectJoin(n *ast.TableRefsClause) error {
	if n == nil || n.TableRefs == nil {
		return nil
	}
	return walkJoin(n.TableRefs)
}

func walkJoin(n ast.ResultSetNode) error {
	switch t := n.(type) {
	case *ast.Join:
		if t.Right != nil {
			return apperr.Unsup("JOIN", "single-table SELECT only in V1; JOIN is V2")
		}
		return walkJoin(t.Left)
	case *ast.TableSource:
		return nil
	case *ast.SelectStmt:
		return apperr.Unsup("subquery", "flatten to a single FROM table")
	default:
		return nil
	}
}

func tableName(n *ast.Join) (string, string, error) {
	src, ok := n.Left.(*ast.TableSource)
	if !ok {
		if j, ok := n.Left.(*ast.Join); ok && n.Right == nil {
			return tableName(j)
		}
		return "", "", apperr.Unsup("complex FROM", "use a single table name")
	}
	tn, ok := src.Source.(*ast.TableName)
	if !ok {
		return "", "", apperr.Unsup("subquery", "FROM must be a virtual table name")
	}
	alias := ""
	if src.AsName.O != "" {
		alias = src.AsName.O
	} else if src.AsName.L != "" {
		alias = src.AsName.L
	}
	return tn.Name.O, alias, nil
}

func convField(f *ast.SelectField) (*Expr, error) {
	if f.WildCard != nil {
		return &Expr{Kind: KStar, Name: "*"}, nil
	}
	e, err := convExpr(f.Expr)
	if err != nil {
		return nil, err
	}
	if f.AsName.O != "" {
		e.Alias = f.AsName.O
	}
	return e, nil
}

func convExpr(n ast.ExprNode) (*Expr, error) {
	if n == nil {
		return nil, apperr.Bad("empty expression")
	}
	switch e := n.(type) {
	case *ast.ColumnNameExpr:
		name := e.Name.Name.O
		if e.Name.Table.O != "" {
			name = e.Name.Name.O
		}
		return Col(name), nil
	case *test_driver.ValueExpr:
		return Lit(datum(e.GetValue())), nil
	case *ast.ParenthesesExpr:
		out, err := convExpr(e.Expr)
		if err != nil {
			return out, err
		}
		return out, nil
	case *ast.BinaryOperationExpr:
		op, err := mapOp(e.Op)
		if err != nil {
			return nil, err
		}
		a, err := convExpr(e.L)
		if err != nil {
			return nil, err
		}
		b, err := convExpr(e.R)
		if err != nil {
			return nil, err
		}
		return Bin(op, a, b), nil
	case *ast.UnaryOperationExpr:
		a, err := convExpr(e.V)
		if err != nil {
			return nil, err
		}
		if e.Op == opcode.Not {
			return &Expr{Kind: KUnary, Op: "NOT", Kids: []*Expr{a}}, nil
		}
		if e.Op == opcode.Minus {
			return Bin(OpSub, Lit(types.VInt(0)), a), nil
		}
		return nil, apperr.Unsup("unary "+e.Op.String(), "supported: NOT, unary minus")
	case *ast.IsNullExpr:
		a, err := convExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: KIsNull, Kids: []*Expr{a}, Not: e.Not}, nil
	case *ast.PatternInExpr:
		if e.Sel != nil {
			return nil, apperr.Unsup("IN subquery", "use IN (literal list)")
		}
		a, err := convExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		kids := []*Expr{a}
		for _, x := range e.List {
			k, err := convExpr(x)
			if err != nil {
				return nil, err
			}
			kids = append(kids, k)
		}
		return &Expr{Kind: KIn, Kids: kids, Not: e.Not}, nil
	case *ast.BetweenExpr:
		a, err := convExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		lo, err := convExpr(e.Left)
		if err != nil {
			return nil, err
		}
		hi, err := convExpr(e.Right)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: KBetween, Kids: []*Expr{a, lo, hi}, Not: e.Not}, nil
	case *ast.PatternLikeOrIlikeExpr:
		a, err := convExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		p, err := convExpr(e.Pattern)
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: KLike, Kids: []*Expr{a, p}, Not: e.Not}, nil
	case *ast.FuncCastExpr:
		a, err := convExpr(e.Expr)
		if err != nil {
			return nil, err
		}
		t, err := types.Parse(e.Tp.String())
		if err != nil {
			t = types.String
		}
		return &Expr{Kind: KCast, Kids: []*Expr{a}, CastTo: t}, nil
	case *ast.AggregateFuncExpr:
		fn := strings.ToUpper(e.F)
		switch fn {
		case "COUNT", "SUM", "AVG", "MIN", "MAX":
		default:
			return nil, apperr.Unsup("aggregate "+fn, "supported: COUNT/SUM/AVG/MIN/MAX")
		}
		ex := &Expr{Kind: KAgg, AggFn: fn, Distinct: e.Distinct}
		if len(e.Args) == 0 {
			ex.Name = "*"
			return ex, nil
		}
		if _, ok := e.Args[0].(*ast.ColumnNameExpr); ok || true {
			k, err := convExpr(e.Args[0])
			if err != nil {
				return nil, err
			}
			if k.Kind == KStar || (k.Kind == KCol && k.Name == "*") {
				ex.Name = "*"
			}
			ex.Kids = []*Expr{k}
		}
		return ex, nil
	case *ast.SubqueryExpr:
		return nil, apperr.Unsup("subquery", "flatten the query")
	case *ast.ExistsSubqueryExpr:
		return nil, apperr.Unsup("EXISTS", "not supported")
	case *ast.FuncCallExpr:
		return nil, apperr.Unsup("function "+e.FnName.O, "supported functions: CAST and aggregates")
	case *ast.WindowFuncExpr:
		return nil, apperr.Unsup("window function (OVER)", "use GROUP BY")
	default:
		if ve, ok := n.(ast.ValueExpr); ok {
			return Lit(datum(ve.GetValue())), nil
		}
		return nil, apperr.Unsup(fmt.Sprintf("expr %T", n), "see SQL capability matrix")
	}
}

func mapOp(op opcode.Op) (BinOp, error) {
	switch op {
	case opcode.EQ:
		return OpEq, nil
	case opcode.NE:
		return OpNe, nil
	case opcode.LT:
		return OpLt, nil
	case opcode.LE:
		return OpLe, nil
	case opcode.GT:
		return OpGt, nil
	case opcode.GE:
		return OpGe, nil
	case opcode.LogicAnd:
		return OpAnd, nil
	case opcode.LogicOr:
		return OpOr, nil
	case opcode.Plus:
		return OpAdd, nil
	case opcode.Minus:
		return OpSub, nil
	case opcode.Mul:
		return OpMul, nil
	case opcode.Div:
		return OpDiv, nil
	default:
		return "", apperr.Unsup("operator "+op.String(), "supported comparisons and + - * / AND OR")
	}
}

func datum(v any) types.Value {
	if v == nil {
		return types.Null(types.String)
	}
	switch x := v.(type) {
	case int64:
		return types.VInt(x)
	case uint64:
		return types.VInt(int64(x))
	case int:
		return types.VInt(int64(x))
	case float64:
		return types.VFloat(x)
	case bool:
		return types.VBool(x)
	case string:
		return types.VStr(x)
	default:
		return types.VStr(fmt.Sprint(x))
	}
}

func constInt(n ast.ExprNode) (int64, error) {
	e, err := convExpr(n)
	if err != nil {
		return 0, err
	}
	if e.Kind != KLit {
		return 0, apperr.Bad("LIMIT/OFFSET must be a constant integer")
	}
	if e.Lit.Type == types.Int64 {
		return e.Lit.I, nil
	}
	i, err := strconv.ParseInt(e.Lit.String(), 10, 64)
	if err != nil {
		return 0, apperr.Bad("LIMIT/OFFSET must be a constant integer")
	}
	return i, nil
}

func hasWindow(s *ast.SelectStmt) bool {
	if s.Fields == nil {
		return false
	}
	for _, f := range s.Fields.Fields {
		if _, ok := f.Expr.(*ast.WindowFuncExpr); ok {
			return true
		}
	}
	return false
}

func collectAggs(pl *Plan) {
	var walk func(*Expr)
	walk = func(e *Expr) {
		if e == nil {
			return
		}
		if e.Kind == KAgg {
			alias := e.Alias
			if alias == "" {
				if e.Name == "*" || (len(e.Kids) == 1 && e.Kids[0].Kind == KStar) {
					alias = e.AggFn + "(*)"
				} else if len(e.Kids) == 1 && e.Kids[0].Kind == KCol {
					alias = e.AggFn + "(" + e.Kids[0].Name + ")"
				} else {
					alias = e.AggFn
				}
			}
			arg := (*Expr)(nil)
			if len(e.Kids) > 0 {
				arg = e.Kids[0]
			}
			pl.Aggs = append(pl.Aggs, AggSpec{Fn: e.AggFn, Arg: arg, Distinct: e.Distinct, Alias: alias})
			e.Alias = alias
		}
		for _, k := range e.Kids {
			walk(k)
		}
	}
	for _, p := range pl.Projects {
		walk(p)
	}
	walk(pl.Having)
}
