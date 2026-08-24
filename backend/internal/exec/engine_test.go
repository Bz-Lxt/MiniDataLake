package exec

import (
	"context"
	"testing"

	"minidatalake/internal/sqlplan"
	"minidatalake/internal/storage"
	"minidatalake/internal/types"
)

func sample() *storage.Table {
	city := storage.BuildStr([]string{"bj", "sh", "bj", "gz", "sh"}, nil)
	age := storage.NewInt64([]int64{10, 20, 30, 40, 18}, storage.NewBitmap(5))
	return &storage.Table{Name: "users", Rows: 5, Status: "READY", Cols: []*storage.Column{
		{Meta: storage.ColumnMeta{Name: "city", Type: types.String}, Vec: city},
		{Meta: storage.ColumnMeta{Name: "age", Type: types.Int64}, Vec: age},
	}}
}

func TestGroupByAvg(t *testing.T) {
	pl, err := sqlplan.Parse("SELECT city, COUNT(*) AS c, AVG(age) FROM users WHERE age > 18 GROUP BY city")
	if err != nil {
		t.Fatal(err)
	}
	tbl := sample()
	if err := sqlplan.Bind(pl, tbl); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), tbl, pl, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows < 1 {
		t.Fatal("no rows")
	}
}

func TestLimitStopsScan(t *testing.T) {
	pl, err := sqlplan.Parse("SELECT * FROM users LIMIT 2")
	if err != nil {
		t.Fatal(err)
	}
	tbl := sample()
	_ = sqlplan.Bind(pl, tbl)
	res, err := Run(context.Background(), tbl, pl, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows != 2 {
		t.Fatal(res.Rows)
	}
	if res.Scanned > 4 {
		t.Fatalf("scanned too many: %d", res.Scanned)
	}
}

func TestUnsupported(t *testing.T) {
	if _, err := sqlplan.Parse("WITH x AS (SELECT 1) SELECT * FROM x"); err == nil {
		t.Fatal("expected unsupported")
	}
}

func TestNullThreeValued(t *testing.T) {
	pl, err := sqlplan.Parse("SELECT age FROM users WHERE age > 100")
	if err != nil {
		t.Fatal(err)
	}
	tbl := sample()
	_ = sqlplan.Bind(pl, tbl)
	res, err := Run(context.Background(), tbl, pl, 16)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows != 0 {
		t.Fatal(res.Rows)
	}
}
