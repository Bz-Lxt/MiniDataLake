package types

import "testing"

func TestInferAndParse(t *testing.T) {
	if InferSample("42") != Int64 {
		t.Fatal(InferSample("42"))
	}
	if InferSample("3.14") != Float64 {
		t.Fatal()
	}
	if InferSample("true") != Bool {
		t.Fatal()
	}
	v, ok := ParseCell("2026-08-22 12:00:00", Timestamp)
	if !ok || v.Null {
		t.Fatal(v, ok)
	}
	if Ident("sales-2024 Q1") != "sales_2024_q1" {
		t.Fatal(Ident("sales-2024 Q1"))
	}
}

func TestCompareNulls(t *testing.T) {
	if Compare(Null(Int64), VInt(1)) >= 0 {
		t.Fatal("nulls first")
	}
}
