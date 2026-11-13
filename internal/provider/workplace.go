package provider

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// gitSaidRe is whether the agent's folder is in a git repository, as the
// agent says it beside the folder: Claude Code's " - Is a git repository:
// false" (2.1.296; "Is directory a git repo: Yes" before), OpenCode's
// "  Is directory a git repo: yes".
var gitSaidRe = regexp.MustCompile(`(?mi)^\s*(?:- )?Is (?:a git repository|directory a git repo): (true|false|yes|no)\s*$`)

// GitSaid is the last "yes" or "no" an agent's text says of its folder
// being in a git repository, "" when it says neither.
func GitSaid(text string) string {
	m := gitSaidRe.FindAllStringSubmatch(text, -1)
	if len(m) == 0 {
		return ""
	}
	switch strings.ToLower(m[len(m)-1][1]) {
	case "true", "yes":
		return "yes"
	}
	return "no"
}

// gitOnDiskFor is how long GitOnDisk keeps an answer: the gateway asks on
// every request whose agent names its folder but not whether it is in a
// repository, and reading up the tree is syscalls on that request's way
// (a slow disk, a network folder). A folder made a repository meanwhile is
// seen within it; a folder that can't be read is asked again at once.
var gitOnDiskFor = 30 * time.Second

var gitOnDiskCache = struct {
	sync.Mutex
	m map[string]gitOnDiskAnswer
}{m: map[string]gitOnDiskAnswer{}}

type gitOnDiskAnswer struct {
	git string
	at  time.Time
}

// forgetGitOnDisk drops what GitOnDisk remembers, for a test that makes a
// folder a repository between two asks.
func forgetGitOnDisk() {
	gitOnDiskCache.Lock()
	gitOnDiskCache.m = map[string]gitOnDiskAnswer{}
	gitOnDiskCache.Unlock()
}

// GitOnDisk says whether dir, a folder on this computer, is in a git
// repository as git finds one from there: the nearest .git in it or in a
// folder above it that git takes (GitAt). A .git folder git doesn't take,
// an empty one say, is passed over as git passes it over, so an empty
// /tmp/.git doesn't make every folder under /tmp a repository (#3675).
// "" when dir isn't a folder here (an agent on another computer, through
// a shared magpie) or a .git on the way can't be read (another user's,
// through a shared magpie): unknown, never "no".
func GitOnDisk(dir string) string {
	if !filepath.IsAbs(dir) {
		return ""
	}
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	gitOnDiskCache.Lock()
	if a, ok := gitOnDiskCache.m[dir]; ok && time.Since(a.at) < gitOnDiskFor {
		gitOnDiskCache.Unlock()
		return a.git
	}
	gitOnDiskCache.Unlock()
	git := gitOnDiskRead(dir)
	// an answer is kept only as long as it is asked for; the map is swept
	// when it grows, so a session through many folders doesn't hold them
	gitOnDiskCache.Lock()
	if len(gitOnDiskCache.m) >= 512 {
		for k, a := range gitOnDiskCache.m {
			if time.Since(a.at) >= gitOnDiskFor {
				delete(gitOnDiskCache.m, k)
			}
		}
		if len(gitOnDiskCache.m) >= 1024 {
			gitOnDiskCache.m = map[string]gitOnDiskAnswer{}
		}
	}
	gitOnDiskCache.m[dir] = gitOnDiskAnswer{git: git, at: time.Now()}
	gitOnDiskCache.Unlock()
	return git
}

// gitOnDiskRead is GitOnDisk's walk up the folder, uncached.
func gitOnDiskRead(dir string) string {
	for d := dir; ; {
		git, _, err := GitAt(filepath.Join(d, ".git"))
		if err != nil {
			return ""
		}
		if git != "" {
			return git
		}
		up := filepath.Dir(d)
		if up == d {
			return "no"
		}
		d = up
	}
}

// GitAt is what the .git at path is to git's own search for a repository
// (repo_discovery_find_dir in git 2.56's setup.c): "yes" for a folder git
// takes for a repository (gitDir), or a file naming one ("gitdir: …", as
// a linked worktree's or a submodule's is), with the folder that holds
// the repository's config, objects and refs; "no" for any other file,
// where git stops with "not a git repository"; "" for nothing to go by,
// no .git or a folder git passes over. An error is a .git it couldn't
// read.
func GitAt(path string) (git, common string, err error) {
	fi, err := os.Stat(path)
	if absent(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if fi.IsDir() {
		if common, err = gitDir(path); common == "" {
			return "", "", err
		}
		return "yes", common, nil
	}
	if !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
		return "no", "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	to, ok := strings.CutPrefix(strings.TrimRight(string(b), "\r\n"), "gitdir: ")
	if !ok || to == "" {
		return "no", "", nil
	}
	if !filepath.IsAbs(to) {
		to = filepath.Join(filepath.Dir(path), to)
	}
	switch common, err = gitDir(to); {
	case err != nil:
		return "", "", err
	case common == "":
		return "no", "", nil
	}
	return "yes", common, nil
}

// gitDir is the folder that holds the repository dir is the git directory
// of, as git takes one (is_git_directory): a HEAD git takes (gitHead), and
// objects and refs folders in the folder its commondir names when it has
// one (a linked worktree's), else in dir. "" when git doesn't take dir for
// one.
func gitDir(dir string) (string, error) {
	if ok, err := gitHead(filepath.Join(dir, "HEAD")); !ok {
		return "", err
	}
	common := dir
	switch b, err := os.ReadFile(filepath.Join(dir, "commondir")); {
	case err == nil:
		common = strings.TrimRight(string(b), "\r\n")
		if !filepath.IsAbs(common) {
			common = filepath.Join(dir, common)
		}
	case !absent(err):
		return "", err
	}
	for _, sub := range []string{"objects", "refs"} {
		fi, err := os.Stat(filepath.Join(common, sub))
		if absent(err) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if !fi.IsDir() {
			return "", nil
		}
	}
	return common, nil
}

// gitHead is whether the HEAD at path is one git takes
// (validate_headref): a link into refs/, or a file that starts with
// "ref: refs/" or with a commit's id.
func gitHead(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if absent(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		to, err := os.Readlink(path)
		// Windows reads a link's target back with its own separator,
		// "refs\heads\main", where git wrote "refs/heads/main"
		return strings.HasPrefix(filepath.ToSlash(to), "refs/"), err
	}
	if !fi.Mode().IsRegular() {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 255))
	if err != nil {
		return false, err
	}
	if ref, ok := bytes.CutPrefix(b, []byte("ref:")); ok && bytes.HasPrefix(bytes.TrimLeft(ref, " \t\n\v\f\r"), []byte("refs/")) {
		return true, nil
	}
	if len(b) < 40 {
		return false, nil
	}
	_, err = hex.DecodeString(string(b[:40]))
	return err == nil, nil
}

// absent is whether err says a path isn't there, rather than that it
// couldn't be read.
func absent(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}
