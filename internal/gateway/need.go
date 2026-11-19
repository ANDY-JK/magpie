package gateway

// need is what a request asks of a model's abilities (capability.go's
// ModelCaps): whether it must call a tool, read a picture and stream its
// answer. It arrives with the request's body facts (parse_memo.go), and it
// is why a candidate is held out of the plan (capOrdered), what the score's
// ability dimension weighs (routes.Seat.Ability) and what a pool routed by
// score offers the background probes (capprobe.go).
type need struct {
	tools, vision, stream bool
}

// empty says the request asks nothing of the model's abilities: no ability
// is weighed, no candidate held out, nothing probed.
func (n need) empty() bool { return !n.tools && !n.vision && !n.stream }

// needOf is need of a parsed request: tools when it carries any, stream
// when it asked for one. vision is read off the body whole (readFacts), not
// off the parse, which a request read for facts alone leaves out for an
// assistant's turns and a tool's pictures.
func needOf(req *Request) need {
	if req == nil {
		return need{}
	}
	return need{tools: len(req.Tools) > 0, stream: req.Stream}
}

// lackingCaps is the tags nd needs that c's provider's capabilities test
// answered "no" for (Provider.Caps): what capOrdered holds the candidate
// out of the plan for. A tag not settled says nothing and holds nobody out.
func lackingCaps(c candidate, nd need) []string {
	caps := c.p.Caps[c.model]
	var out []string
	if nd.tools && caps.Tools == "no" {
		out = append(out, "tools")
	}
	if nd.vision && caps.Vision == "no" {
		out = append(out, "vision")
	}
	if nd.stream && caps.Stream == "no" {
		out = append(out, "stream")
	}
	return out
}

// abilityOf is what c's provider's capabilities test says of what nd asks,
// 0–1: each tag wanted counts, a settled yes a whole one, one not settled
// half — untested is neither rewarded nor punished — and a settled no
// nothing, the candidate those hold out of the plan unless every one of
// them is (capOrdered). A need that asks nothing is met whole: 1, every
// seat alike, no signal to the score.
func abilityOf(c candidate, nd need) float64 {
	if nd.empty() {
		return 1
	}
	caps := c.p.Caps[c.model]
	sum, n := 0.0, 0
	for _, t := range []struct {
		want bool
		said string
	}{{nd.tools, caps.Tools}, {nd.vision, caps.Vision}, {nd.stream, caps.Stream}} {
		if !t.want {
			continue
		}
		n++
		switch t.said {
		case "yes":
			sum++
		case "no":
		default:
			sum += 0.5
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// capOrdered is the plan's hard exclusion by what the request needs and a
// capabilities test settled (Provider.Caps): a candidate a wanted tag is
// probed "no" for is left out of it, its row among the plan's left saying
// why (Weighed.Lacks), the way one the user barred is — never filtered
// inside an order. When every candidate of the plan is one of those, none
// is left out: the plan answers with them, the score's ability dimension
// holding each behind what an able one would score. out and pl.order stand
// in each other's place through a plan (restLast, sinkPlanned), so a
// candidate left out takes its row with it and the rest keep theirs; rows
// beyond the candidates ride at the back as they were.
func capOrdered(out []candidate, pl planned, nd need) ([]candidate, planned) {
	if nd.empty() || len(out) == 0 {
		return out, pl
	}
	ready := make([]candidate, 0, len(out))
	wReady := make([]Weighed, 0, len(out))
	var lacks []candidate
	var wLacks []Weighed
	for i, c := range out {
		if tags := lackingCaps(c, nd); len(tags) > 0 {
			w := pl.order[i]
			w.Lacks = tags
			lacks, wLacks = append(lacks, c), append(wLacks, w)
			continue
		}
		ready, wReady = append(ready, c), append(wReady, pl.order[i])
	}
	if len(ready) == 0 {
		return out, pl
	}
	if len(pl.order) > len(out) {
		wReady = append(wReady, pl.order[len(out):]...)
	}
	pl.order = wReady
	pl.left = append(pl.left, wLacks...)
	return ready, pl
}
