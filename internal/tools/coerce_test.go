package tools

import "testing"

func TestArgInt(t *testing.T) {
	m := map[string]any{"n": float64(7)}
	n, ok := ArgInt(m, "n")
	if !ok || n != 7 {
		t.Fatalf("got %d ok=%v", n, ok)
	}
	m2 := map[string]any{"n": 12}
	n, ok = ArgInt(m2, "n")
	if !ok || n != 12 {
		t.Fatalf("int: got %d ok=%v", n, ok)
	}
}

func TestArgBool(t *testing.T) {
	v, ok := ArgBool(map[string]any{"x": true}, "x")
	if !ok || !v {
		t.Fatal(v, ok)
	}
	v, ok = ArgBool(map[string]any{"x": false}, "x")
	if !ok || v {
		t.Fatal(v, ok)
	}
	_, ok = ArgBool(map[string]any{}, "x")
	if ok {
		t.Fatal("expected missing")
	}
}
