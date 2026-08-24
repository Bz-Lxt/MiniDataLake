package main

import (
	"os"

	"github.com/parquet-go/parquet-go"
)

type Rec struct {
	SKU   string  `parquet:"sku"`
	Qty   int64   `parquet:"qty"`
	Price float64 `parquet:"price"`
	City  string  `parquet:"city"`
}

func main() {
	out := "../testdata/samples/sales.parquet"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := parquet.NewGenericWriter[Rec](f)
	rows := []Rec{
		{"A1", 3, 12.5, "beijing"},
		{"A1", 1, 12.5, "beijing"},
		{"B9", 8, 4.2, "shanghai"},
		{"C3", 2, 99, "shenzhen"},
	}
	if _, err := w.Write(rows); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
}
