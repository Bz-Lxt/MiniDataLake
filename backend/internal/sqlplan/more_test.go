package sqlplan

import (
	"testing"

	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func TestMoreSQLShapes(t *testing.T) {
	cases := []string{
		"SELECT * FROM users WHERE age >= 18 AND city <> 'x' OR city IS NULL",
		"SELECT CAST(age AS CHAR) FROM users",
		"SELECT age+1, age-1, age*2, age/2 FROM users",
		"SELECT COUNT(DISTINCT city), SUM(age) FROM users",
		"EXPLAIN SELECT * FROM users LIMIT 1",
		"SELECT city FROM users WHERE city NOT LIKE '%z%' AND age NOT BETWEEN 0 AND 1",
		"SELECT city FROM t u",
	}
	for _, s := range cases {
		if _, err := Parse(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func TestExpandAndSuggest(t *testing.T) {
	tbl := &storage.Table{Name: "users", Cols: []*storage.Column{
		{Meta: storage.ColumnMeta{Name: "city", Type: types.String}},
		{Meta: storage.ColumnMeta{Name: "age", Type: types.Int64}},
	}}
	pl, err := Parse("SELECT * FROM users")
	if err != nil {
		t.Fatal(err)
	}
	ExpandStar(pl, tbl)
	if len(pl.Projects) != 2 {
		t.Fatal(len(pl.Projects))
	}
	if suggest("cty", tbl) != "city" {
		t.Fatal(suggest("cty", tbl))
	}
	if err := Bind(pl, tbl); err != nil {
		t.Fatal(err)
	}
}

func TestExprHelpers(t *testing.T) {
	e := Bin(OpAdd, Col("a"), Lit(types.VInt(1)))
	if len(e.Columns()) != 1 || e.IsAgg() {
		t.Fatal()
	}
	agg := &Expr{Kind: KAgg, AggFn: "COUNT", Kids: []*Expr{Col("a")}}
	if !agg.IsAgg() {
		t.Fatal()
	}
}
