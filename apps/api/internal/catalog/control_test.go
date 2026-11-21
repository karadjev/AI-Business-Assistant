package catalog

import "testing"

func TestHealth(t *testing.T) {
	if Health(HealthInput{}) != nil {
		t.Fatal("empty catalogue cannot have a score")
	}
	for _, v := range []struct {
		in   HealthInput
		want int
	}{{HealthInput{Total: 10, Complete: 10, Priced: 10, Known: 10, Fresh: 10}, 100}, {HealthInput{Total: 10}, 20}, {HealthInput{Total: 10, Complete: 5, Priced: 5, Known: 5, Fresh: 5, Conflicts: 5, ImportRows: 10, ImportErrors: 5}, 50}, {HealthInput{Total: 1, Conflicts: 100, ImportRows: 1, ImportErrors: 100}, 0}} {
		if got := *Health(v.in); got != v.want {
			t.Fatalf("health %+v = %d want%d", v.in, got, v.want)
		}
	}
}

func TestControlQueryBounds(t *testing.T) {
	for _, q := range []ControlQuery{{Page: -1}, {ImportPage: -1}, {Page: 10001}, {ImportPage: 10001}, {Query: string(make([]byte, 201))}, {Filter: "UNKNOWN_SQL"}} {
		if q.Validate() == nil {
			t.Fatal("invalid control query", q)
		}
	}
	q := ControlQuery{}
	if q.Validate() != nil || q.Page != 1 || q.ImportPage != 1 {
		t.Fatal("default pagination")
	}
}
