package main

import (
	"encoding/csv"
	"fmt"
	"os"
)

func main() {
	n := 20000
	out := "../testdata/bench_20k.csv"
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &n)
	}
	if len(os.Args) > 2 {
		out = os.Args[2]
	}
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"id", "age", "city", "channel", "score", "flag", "region", "sku", "qty", "price", "day", "note"})
	cities := []string{"beijing", "shanghai", "shenzhen", "guangzhou", "chengdu", "hangzhou"}
	ch := []string{"web", "app", "retail"}
	for i := 0; i < n; i++ {
		_ = w.Write([]string{
			fmt.Sprint(i), fmt.Sprint(18 + i%50), cities[i%len(cities)], ch[i%3],
			fmt.Sprintf("%.2f", float64(i%100)/3), fmt.Sprint(i%2 == 0),
			cities[(i/2)%len(cities)], fmt.Sprintf("SKU-%03d", i%40),
			fmt.Sprint(1 + i%8), fmt.Sprintf("%.2f", 9.9+float64(i%20)),
			fmt.Sprintf("2026-01-%02d", 1+i%28), "ok",
		})
	}
	w.Flush()
}
