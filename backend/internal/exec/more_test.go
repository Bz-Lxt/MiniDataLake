package exec

import (
	"context"
	"testing"

	"minidatalake/internal/sqlplan"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func runSQL(t *testing.T, sql string) *Result {
	t.Helper()
	pl, err := sqlplan.Parse(sql)
	if err != nil {
		t.Fatal(err)
	}
	tbl := sample()
	if err := sqlplan.Bind(pl, tbl); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), tbl, pl, 3)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestOrderHavingDistinctLike(t *testing.T) {
	r := runSQL(t, "SELECT city, COUNT(*) AS c FROM users GROUP BY city HAVING c >= 2 ORDER BY c DESC")
	if r.Rows < 2 {
		t.Fatal(r.Rows)
	}
	r = runSQL(t, "SELECT DISTINCT city FROM users WHERE city LIKE 'b%'")
	if r.Rows != 1 {
		t.Fatal(r.Rows)
	}
	r = runSQL(t, "SELECT age FROM users WHERE age BETWEEN 20 AND 40 AND city IN ('bj','sh')")
	if r.Rows < 1 {
		t.Fatal(r.Rows)
	}
	r = runSQL(t, "SELECT SUM(age), MIN(age), MAX(age) FROM users")
	if r.Rows != 1 {
		t.Fatal(r.Rows)
	}
	r = runSQL(t, "SELECT * FROM users WHERE age IS NOT NULL LIMIT 1 OFFSET 1")
	if r.Rows != 1 {
		t.Fatal(r.Rows)
	}
}

func TestBinderUnknown(t *testing.T) {
	pl, err := sqlplan.Parse("SELECT nope FROM users")
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlplan.Bind(pl, sample()); err == nil {
		t.Fatal("expected semantic error")
	}
}

func TestTablePreviewSlice(t *testing.T) {
	tbl := sample()
	if len(tbl.Preview(2)) != 2 {
		t.Fatal()
	}
	vs := tbl.Slice([]string{"age"}, 1, 3)
	if vs[0].Len() != 2 || vs[0].Get(0).I != 20 {
		t.Fatal(vs[0].Get(0))
	}
	if tbl.MemBytes() <= 0 {
		t.Fatal(tbl.MemBytes())
	}
	if _, _, ok := tbl.ColByName("city"); !ok {
		t.Fatal()
	}
}

func TestEmptyAgg(t *testing.T) {
	empty := &storage.Table{Name: "users", Rows: 0, Status: "READY", Cols: []*storage.Column{
		{Meta: storage.ColumnMeta{Name: "age", Type: types.Int64}, Vec: storage.NewInt64(nil, storage.NewBitmap(0))},
		{Meta: storage.ColumnMeta{Name: "city", Type: types.String}, Vec: storage.BuildStr(nil, nil)},
	}}
	pl, _ := sqlplan.Parse("SELECT COUNT(*) FROM users")
	_ = sqlplan.Bind(pl, empty)
	res, err := Run(context.Background(), empty, pl, 8)
	if err != nil || res.Rows != 1 {
		t.Fatal(err, res)
	}
}
