package clock

import "testing"

func TestBeijingOffset(t *testing.T) {
	_, off := Now().Zone()
	if off != 8*3600 {
		t.Fatalf("offset %d", off)
	}
	if Format(Now()) == "" {
		t.Fatal("empty format")
	}
}
