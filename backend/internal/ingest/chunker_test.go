package ingest

import (
	"bytes"
	"strings"
	"testing"
)

type ra struct{ b []byte }

func (r ra) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.b)) {
		return 0, nil
	}
	n := copy(p, r.b[off:])
	return n, nil
}

func TestSplitQuotedNewline(t *testing.T) {
	data := []byte("h1,h2\n\"a\nb\",2\n3,4\n")
	ch, err := SplitCSV(ra{data}, int64(len(data)), 8, ',')
	if err != nil {
		t.Fatal(err)
	}
	var all []byte
	for _, c := range ch {
		all = append(all, c.Data...)
	}
	if !bytes.Contains(all, []byte("\"a\nb\"")) {
		t.Fatalf("lost quoted newline: %q chunks=%d", all, len(ch))
	}
}

func TestParseCSVLineEscapes(t *testing.T) {
	got := ParseCSVLine([]byte(`"a""b",x`), ',')
	if len(got) != 2 || got[0] != `a"b` {
		t.Fatal(got)
	}
}

func TestCRLFAndNoTrailingNL(t *testing.T) {
	data := []byte("a,b\r\n1,2")
	ch, err := SplitCSV(ra{data}, int64(len(data)), 3, ',')
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, c := range ch {
		joined += string(c.Data)
	}
	if !strings.Contains(joined, "1,2") {
		t.Fatal(joined)
	}
}

func TestTableName(t *testing.T) {
	got := TableName("sales-2024 Q1.csv", map[string]bool{})
	if got != "sales_2024_q1" {
		t.Fatal(got)
	}
	got2 := TableName("sales-2024 Q1.csv", map[string]bool{"sales_2024_q1": true})
	if got2 != "sales_2024_q1_2" {
		t.Fatal(got2)
	}
}
