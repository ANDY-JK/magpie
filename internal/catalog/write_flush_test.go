package catalog

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/steady"
)

// The two catalogs written whole — models.dev's, and a vendor's own list —
// are flushed to the disk before they are renamed over the one kept: a flush
// that fails leaves the old file as it was, with no temp beside it (#1505).
// Written straight over, as before steady.WriteFile, a reader in the window,
// the gateway or an agent could meet a half-written catalog, and a machine
// that went down could leave one at its full length with every byte zero.

// noTempLeft fails the test when p's folder holds a temp file steady.WriteFile
// made for it.
func noTempLeft(t *testing.T, p string) {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "."+filepath.Base(p)+".*"))
	if len(left) > 0 {
		t.Fatalf("a failed flush left %v", left)
	}
}

func TestSyncFlushesCatalogBeforeRenaming(t *testing.T) {
	api, _ := os.ReadFile(claudeAPI10_27)
	labs, _ := os.ReadFile(claudeLabs10_27)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api.json":
			w.Write(api)
		case "/models.json":
			w.Write(labs)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	useCatalogs(t, "", "")
	was := modelsDevURL
	modelsDevURL = srv.URL + "/api.json"
	t.Cleanup(func() { modelsDevURL = was; labsTried.Store(false) })

	if err := Sync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	kept, err := os.ReadFile(CachePath())
	if err != nil || !bytes.Equal(kept, api) {
		t.Fatalf("catalog written = %d bytes, %v", len(kept), err)
	}

	defer func() { steady.Sync = (*os.File).Sync }()
	steady.Sync = func(*os.File) error { return errors.New("the disk went away") }
	if err := Sync(context.Background()); err == nil {
		t.Fatal("a catalog that couldn't be flushed reported as written")
	}
	now, err := os.ReadFile(CachePath())
	if err != nil || !bytes.Equal(now, kept) {
		t.Fatalf("a failed flush changed the catalog to %d bytes, %v", len(now), err)
	}
	noTempLeft(t, CachePath())
}

func TestSaveLiveFlushesBeforeRenaming(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(Reset)
	models := []Model{{ID: "claude-sonnet-5"}, {ID: "claude-opus-5-5"}}
	if err := SaveLiveSides("flush", "https://chat/v1", models, nil); err != nil {
		t.Fatalf("first save: %v", err)
	}
	kept, err := os.ReadFile(LivePath("flush"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { steady.Sync = (*os.File).Sync }()
	steady.Sync = func(*os.File) error { return errors.New("the disk went away") }
	if err := SaveLiveSides("flush", "https://chat/v1", append(models, Model{ID: "claude-haiku-5-5"}), nil); err == nil {
		t.Fatal("a model list that couldn't be flushed reported as saved")
	}
	now, err := os.ReadFile(LivePath("flush"))
	if err != nil || !bytes.Equal(now, kept) {
		t.Fatalf("a failed flush changed the list to %d bytes, %v", len(now), err)
	}
	noTempLeft(t, LivePath("flush"))
}
