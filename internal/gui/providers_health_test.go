package gui

import (
	"encoding/json"
	"testing"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

// A provider's row says how its models are answering: health is the
// gateway's aggregation, said only when a model of its has a record — a
// provider nothing has asked yet leaves the key out, not a zero badge.
func TestProviderHealthInProviderState(t *testing.T) {
	sandboxHome(t)
	was := providerHealth
	defer func() { providerHealth = was }()
	providerHealth = func(ids []string) gateway.Health {
		if len(ids) != 1 || ids[0] != "relay" {
			t.Fatalf("asked %v, want the one provider", ids)
		}
		return gateway.Health{Worst: 0.2, WorstModel: "m1", Below: 1, Recorded: 4}
	}
	p := provider.Provider{ID: "relay", Name: "Relay", Chat: "https://example.invalid/v1", Key: "sk-test"}
	out := providerInfoWith(p, provider.Listing{}, nil, nil)
	if out.Health == nil || out.Health.Worst != 0.2 || out.Health.WorstModel != "m1" || out.Health.Below != 1 || out.Health.Recorded != 4 {
		t.Fatalf("health: %+v", out.Health)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Health *gateway.Health `json:"health"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Health == nil || got.Health.Worst != 0.2 || got.Health.WorstModel != "m1" || got.Health.Below != 1 || got.Health.Recorded != 4 {
		t.Fatalf("the provider's state: %s", b)
	}

	// nothing recorded: no health key at all
	providerHealth = func([]string) gateway.Health { return gateway.Health{Worst: 1} }
	quiet := providerInfoWith(p, provider.Listing{}, nil, nil)
	if quiet.Health != nil {
		t.Fatalf("a quiet provider says health: %+v", quiet.Health)
	}
	b, err = json.Marshal(quiet)
	if err != nil {
		t.Fatal(err)
	}
	var none map[string]any
	if err := json.Unmarshal(b, &none); err != nil {
		t.Fatal(err)
	}
	if _, ok := none["health"]; ok {
		t.Fatalf("a quiet provider's state carries health: %s", b)
	}
}
