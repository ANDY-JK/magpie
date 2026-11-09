package provider

import (
	"fmt"
	"testing"

	"github.com/yetone/magpie/internal/filememo"
)

// A request to a group looks it up several times (FindGroup for the route,
// its members and the window, GroupFor beside it), and each look copied
// every group and its rules over to read one of them. While a request holds
// the catalog the looks read the groups held, and a look that hands a group
// out still hands one of its own to change, as Groups' are.
func TestGroupLooksShareHeldGroups(t *testing.T) {
	autoRefHome(t)
	if err := SaveGroup(Group{Name: "Mine", Members: []string{"a/x", "b/x"}}); err != nil {
		t.Fatal(err)
	}
	// the groups a user with many keeps: each with members and a rule
	for i := range 40 {
		if err := SaveGroup(Group{Name: fmt.Sprintf("G%d", i), Members: []string{"a/x", "b/x", "c/x"},
			Rules: []Rule{{Use: "a/x", Agents: []string{"claude"}, Folders: []string{"/w"}}}}); err != nil {
			t.Fatal(err)
		}
	}
	defer Hold()()
	defer filememo.Hold()()
	draws := func(p Provider, model string) bool { return false }
	if g, ms, ok := FindGroup(GroupPrefix + "mine"); !ok || len(ms) == 0 || g.ID != "mine" {
		t.Fatal("no group mine")
	}
	if gid, ok := GroupFor("mine"); !ok || gid != GroupPrefix+"mine" {
		t.Fatalf("GroupFor(\"mine\") = %q, %v", gid, ok)
	}
	if g, ms, ok := FindDrawingGroup(GroupPrefix+"mine", draws); !ok || g.ID != "mine" || len(ms) == 0 {
		t.Fatal("no group mine to draw")
	}
	copied := testing.AllocsPerRun(5, func() { Groups() })
	looked := testing.AllocsPerRun(20, func() {
		FindGroup(GroupPrefix + "mine")
		GroupFor("mine")
		FindDrawingGroup(GroupPrefix+"mine", draws)
	})
	if looked*3 > copied {
		t.Fatalf("a FindGroup, a GroupFor and a FindDrawingGroup took %.0f allocations, a copy of the groups %.0f", looked, copied)
	}

	// a look that hands a group out hands one of its own: its caller can
	// change it without writing into the groups held
	g, _, ok := FindGroup(GroupPrefix + "mine")
	if !ok {
		t.Fatal("no group mine")
	}
	g.Members[0] = "changed"
	if h, _ := groupOf(sharedGroups(), "mine"); h.Members[0] == "changed" {
		t.Fatal("FindGroup's caller wrote into the groups held")
	}
}
