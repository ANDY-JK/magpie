package provider

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/yetone/magpie/internal/seal"
)

// providers.json is sealed for the user of the machine it is on: on Windows
// the file on the disk is a DPAPI blob another account of the machine, or
// magpie on another machine, can't read back, and elsewhere the seal is the
// identity, so the file stays the JSON it always was and a backup or a
// sync's mirror of it is unchanged. A generation magpie writes (sealed),
// and a plain one an older magpie or another machine left, read alike.
func TestStoreSealsForTheUser(t *testing.T) {
	providerFileHome(t)
	if err := Save(Provider{ID: "a", Name: "A", Chat: "https://a.example.invalid/v1", Key: "sk-synthetic-a"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if !seal.Sealed(b) {
			t.Fatalf("providers.json is not sealed on Windows: %q", string(bytes.TrimSpace(b)))
		}
		if json.Valid(b) {
			t.Fatal("the sealed blob reads as plain JSON: a copy of it would carry the keys")
		}
	} else {
		if seal.Sealed(b) {
			t.Fatal("off Windows the file is sealed")
		}
		if !json.Valid(b) {
			t.Fatalf("off Windows the file is not the JSON it has always been: %q", b)
		}
	}
	// a read through the package's own way gives the catalog back
	ps, err := Stored()
	if err != nil || len(ps) != 1 || ps[0].ID != "a" {
		t.Fatalf("Stored after the seal: %v %v", ps, err)
	}
	// a second save keeps the first (sealed) generation as the last good:
	// the .bak is a sealed blob now, not plain JSON
	if err := Save(Provider{ID: "b", Name: "B", Chat: "https://b.example.invalid/v1", Key: "sk-synthetic-b"}); err != nil {
		t.Fatal(err)
	}
	if bak, err := os.ReadFile(Path() + ".bak"); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS == "windows" && !seal.Sealed(bak) {
		t.Fatalf("the last good generation is not sealed: %q", string(bytes.TrimSpace(bak)))
	}
	// a broken main file falls back to that sealed last good generation:
	// the recovery reads it back through the seal, not as empty or broken.
	// Without the seal read off in the last good's check, a sealed .bak
	// would read as "doesn't parse" and the file would be unreadable.
	if err := os.WriteFile(Path(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ps, err := Stored(); err != nil || len(ps) != 1 || ps[0].ID != "a" {
		t.Fatalf("a broken main file fell back to its last good (sealed) generation: %v %v", ps, err)
	}
	// and a plain generation, as an older magpie or another machine left
	// one, still reads, and the next write seals it
	if err := os.WriteFile(Path(), []byte(`{"providers":[{"id":"old","name":"Old","chat":"https://old.example.invalid","key":"sk-synthetic-old"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ps, err := Stored(); err != nil || len(ps) != 1 || ps[0].ID != "old" {
		t.Fatalf("a plain generation: %v %v", ps, err)
	}
}

// logins.json's accounts are the sign-ins' credentials (a refresh token in
// the agent's own blob): the same seal, so a file another account of the
// machine reads is unknown, never the credentials, and a plain generation
// of its own still reads.
func TestLoginsSealForTheUser(t *testing.T) {
	providerFileHome(t)
	saveLogins(t, savedLogin{Agent: "codex", User: "a@b.example", Auth: json.RawMessage(`{"refresh_token":"synthetic-refresh"}`)})
	b, err := os.ReadFile(loginsPath())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if !seal.Sealed(b) || json.Valid(b) {
			t.Fatalf("logins.json is not a sealed blob: %q", string(bytes.TrimSpace(b)))
		}
	} else {
		if seal.Sealed(b) || !json.Valid(b) {
			t.Fatalf("off Windows logins.json is not the JSON it has always been: %q", b)
		}
	}
	// and the credentials behind the seal, through the package's own read
	if ls := readLogins(); len(ls) != 1 || !bytes.Contains(ls[0].Auth, []byte("synthetic-refresh")) {
		t.Fatalf("the credentials behind the seal: %v", ls)
	}
	// a plain generation, as an older magpie left one, reads through the
	// same way: written with the same marker a plain file always held
	marker := "synthetic-plain-generation"
	if err := os.WriteFile(loginsPath(), []byte(`[{"agent":"codex","user":"c@d.example","auth":{"refresh_token":"`+marker+`"}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ls := readLogins(); len(ls) != 1 || !bytes.Contains(ls[0].Auth, []byte(marker)) {
		t.Fatalf("the plain generation: %v", ls)
	}
}
