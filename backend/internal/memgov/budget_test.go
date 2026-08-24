package memgov

import "testing"

func TestReserveDeny(t *testing.T) {
	b := New(100, func() []Victim { return []Victim{{Name: "big", Bytes: 80}} })
	if err := b.Reserve(40); err != nil {
		t.Fatal(err)
	}
	if err := b.Reserve(80); err == nil {
		t.Fatal("expected deny")
	}
	b.Release(40)
	if b.Used() != 0 {
		t.Fatal(b.Used())
	}
}
