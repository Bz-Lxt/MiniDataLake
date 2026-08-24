package sqlplan

import "testing"

func TestParseSelectShape(t *testing.T) {
	pl, err := Parse("SELECT city, COUNT(*) AS c, AVG(age) FROM users WHERE age > 18 GROUP BY city HAVING c > 1 ORDER BY c DESC LIMIT 10")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Table != "users" || !pl.HasLimit || pl.Limit != 10 || len(pl.Groups) != 1 {
		t.Fatalf("%+v", pl)
	}
	if len(pl.Aggs) < 2 {
		t.Fatalf("aggs %+v", pl.Aggs)
	}
}

func TestRejectJoinAndCTE(t *testing.T) {
	if _, err := Parse("SELECT * FROM a JOIN b ON a.id=b.id"); err == nil {
		t.Fatal("join")
	}
	if _, err := Parse("INSERT INTO t VALUES (1)"); err == nil {
		t.Fatal("insert")
	}
}

func TestLevenshtein(t *testing.T) {
	if levenshtein("city", "ciyt") > 2 {
		t.Fatal()
	}
}
