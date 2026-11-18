package provider

import "testing"

// DayCalls is kept with the provider, within 0–MaxDayCalls, and 0 takes it
// off; a subscription's is kept with its picks and is every account's of it.
func TestSetDayCalls(t *testing.T) {
	signIn(t)
	if err := Save(Provider{ID: "or", Name: "OpenRouter", Key: "k", Models: []string{"m"}, Chat: "http://127.0.0.1:9/v1"}); err != nil {
		t.Fatal(err)
	}
	if err := SetDayCalls("or", 100); err != nil {
		t.Fatal(err)
	}
	if p, err := Find("or"); err != nil || p.DayCalls != 100 || p.DayCallLimit() != 100 {
		t.Fatalf("after SetDayCalls 100: %+v %v", p, err)
	}
	for _, n := range []int{-1, MaxDayCalls + 1} {
		if err := SetDayCalls("or", n); err == nil {
			t.Fatalf("SetDayCalls %d was taken", n)
		}
	}
	if p, _ := Find("or"); p.DayCalls != 100 {
		t.Fatalf("a refused limit changed it: %d", p.DayCalls)
	}
	p, _ := Find("or")
	p.DayCalls = 500000
	if err := Save(*p); err != nil {
		t.Fatal(err)
	}
	if p, _ := Find("or"); p.DayCalls != MaxDayCalls {
		t.Fatalf("saved past the bound: %d", p.DayCalls)
	}
	p, _ = Find("or")
	p.DayCalls = -5
	if err := Save(*p); err != nil {
		t.Fatal(err)
	}
	if p, _ := Find("or"); p.DayCalls != 0 {
		t.Fatalf("saved below none: %d", p.DayCalls)
	}
	if err := SetDayCalls("or", 0); err != nil {
		t.Fatal(err)
	}
	if p, _ := Find("or"); p.DayCalls != 0 || p.DayCallLimit() != 0 {
		t.Fatalf("off: %d", p.DayCalls)
	}

	if err := SetDayCalls("codex", 30); err != nil {
		t.Fatal(err)
	}
	if s, ok := storedPicks("codex"); !ok || s.DayCalls != 30 {
		t.Fatalf("codex's picks: %+v", s)
	}
	n := 0
	for _, a := range All() {
		if a.ID == "codex" && a.Account != nil {
			n++
			if a.DayCallLimit() != 30 {
				t.Fatalf("codex account %s: limit %d", a.Account.User, a.DayCallLimit())
			}
		}
	}
	if n == 0 {
		t.Fatal("no codex account listed")
	}
}
