package gateway

// The routing history keeps one route a line. A route's Order and Left are
// the candidates it weighed: a busy route has tens of them, and each Weighed
// carries tens of fields. Marshalled whole, a line averaged 26.5 KB and
// reached 46 KB (73 candidates) — the day's file and its gzip are made of
// those lines (F-15).
//
// keptLine writes new lines with a few fields the history's readers never
// use omitted. The GUI replay needs every other field: each feeds a sentence
// of why a candidate went where it did. The dropped fields are shared, busy,
// capBack, creditsFirst and unread — none is read from a route candidate by
// the Routing page (routing client.js) or by the tallies. Keeping the JSON
// name of every retained field (embedding Weighed and shadowing only the
// dropped ones) means a line reads with the same encoding/json path whether
// it was written before this change or after: old lines still carry the
// extra fields and decode exactly as before; in a new line an omitted field
// is its zero value, which is what the reader treats an absent field as.
// The order of Order and Left is untouched, as the GUI shows the tried and
// skipped order.

import (
	"encoding/json"
	"time"
)

// keptWeighed keeps all of Weighed but hides the fields the history's
// readers never use. A same-named field at a shallower depth takes JSON
// precedence over the embedded Weighed's field, so encoding/json leaves it
// out when it is empty (omitempty) — which it always is here.
type keptWeighed struct {
	Weighed
	Shared       bool       `json:"shared,omitempty"`
	Busy         float64    `json:"busy,omitempty"`
	CapBack      *time.Time `json:"capBack,omitempty"`
	CreditsFirst bool       `json:"creditsFirst,omitempty"`
	Unread       string     `json:"unread,omitempty"`
}

type keptRoute struct {
	Route
	Order []keptWeighed `json:"order"`
	Left  []keptWeighed `json:"left,omitempty"`
}

// keptLine marshals r in the slim history form, one line's worth. Order is
// [] for none, as the GUI reads it as a list; Left is written only when it
// has candidates, as before (its omitempty tag).
func keptLine(r Route) ([]byte, error) {
	k := keptRoute{Route: r}
	k.Order = make([]keptWeighed, len(r.Order))
	for i, w := range r.Order {
		k.Order[i] = keptWeighed{Weighed: w}
	}
	if len(r.Left) > 0 {
		k.Left = make([]keptWeighed, len(r.Left))
		for i, w := range r.Left {
			k.Left[i] = keptWeighed{Weighed: w}
		}
	}
	return json.Marshal(k)
}
