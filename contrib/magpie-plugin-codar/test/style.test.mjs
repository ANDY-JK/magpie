// The formatting rules from .editorconfig, pinned so they can't drift back.
//
// .editorconfig states the style — LF, no trailing whitespace, a final newline,
// two-space indentation — but nothing enforces it: a file saved by an editor
// with the wrong settings, or a pasted block with a stray tab or a trailing
// space, slips through and only shows up as diff noise on the next change. CI
// runs this with the rest of the suite, on every platform, so a checkout with
// core.autocrlf=true is caught here too (a CR creeping back in is what
// .gitattributes's eol=lf exists to prevent; the one sure way to know it did
// its job is a test that would have noticed).
//
// Only the rules that are unambiguous from the tree itself are checked. Markdown
// is exempt from the trailing-whitespace rule (.editorconfig says so, and two
// spaces before a newline is a hard break there), and the check is line-based,
// not a parser, so it stays cheap and gives a line number a reader can open.
//
// The files are discovered from the tree rather than listed. A hardcoded list
// has two failures this file is meant to catch: a new file is quietly outside
// the rules, and the list is easy to write without the file that carries it.
// Walking the tree means a file is covered the moment it lands. Dot-directories
// are skipped: .git and .claude (worktrees of this repo, which would otherwise
// be scanned a second time under a different checkout) are not this tree's
// text; .github is scanned, because a workflow file that came back as CRLF is
// exactly what .gitattributes's eol=lf exists to prevent, and CI runs the suite
// on Windows.

import assert from "node:assert/strict"
import { test } from "node:test"
import { existsSync, readFileSync, readdirSync } from "node:fs"
import { join } from "node:path"
import { fileURLToPath } from "node:url"

const here = fileURLToPath(new URL(".", import.meta.url))
const root = join(here, "..")

const walk = (dir, ext, out = []) => {
  for (const entry of readdirSync(join(root, dir), { withFileTypes: true })) {
    // The dot-directories that are scanned: CI's configuration is this
    // tree's text (a CRLF workflow file is a Windows problem the LF rule
    // exists for), while .git and .claude are not.
    if (entry.name === "node_modules") continue
    if (entry.name.startsWith(".") && entry.name !== ".github") continue
    const rel = dir ? `${dir}/${entry.name}` : entry.name
    if (entry.isDirectory()) walk(rel, ext, out)
    else if (entry.name.endsWith(ext)) out.push(rel)
  }
  return out
}

// text is every file a text rule in .editorconfig names: the JS and markdown
// the suite was the only place for, plus the JSON and YAML the metadata and CI
// configuration are written in. A CRLF workflow file is what .gitattributes's
// eol=lf exists to prevent, and it was outside every check.
const TEXT = [...walk("", ".mjs"), ...walk("", ".md"), ...walk("", ".json"), ...walk("", ".yml"), ...walk("", ".yaml")]
const JS = TEXT.filter((f) => f.endsWith(".mjs"))
const MD = TEXT.filter((f) => f.endsWith(".md"))

const read = (rel) => readFileSync(join(root, rel), "utf8")

const sortedFiles = (files) => [...files].sort()

test("the tree walk finds the files it is meant to guard", () => {
  // A discovery list that comes back empty or truncated makes every check
  // below pass without looking at anything — the same "denominator is zero,
  // call it ok" failure the runtime probes guard against. Pin the floor and
  // the files that must be in it, including this one.
  assert.ok(JS.length >= 6, `the walk found only ${JS.length} .mjs files`)
  for (const rel of ["index.mjs", "lib/internal.mjs", "lib/catalog.mjs", "scripts/catalog.mjs", "test/plugin.test.mjs", "test/style.test.mjs"]) {
    assert.ok(JS.includes(rel), `the walk missed ${rel}`)
  }
  assert.ok(MD.length >= 2, `the walk found only ${MD.length} .md files`)
  // The suite must pass from the npm tarball as much as from a checkout, so
  // the required set is only what ships: CONTRIBUTING.md stays out of the
  // package (it is developer-facing), and would make a tarball test red for no
  // reason even though the checkout walk still finds it.
  assert.ok(MD.includes("README.md") && MD.includes("CHANGELOG.md"), "the walk missed a documentation file")
  // JSON and YAML are covered where they exist: package.json ships, and the
  // tarball carries it. .github is a checkout's own (it is deliberately not
  // in `files`), so the CI configuration is required only where the
  // directory is there to walk — a tarball must not fail on a file it was
  // never meant to carry.
  assert.ok(TEXT.includes("package.json"), "the walk missed package.json")
  if (existsSync(join(root, ".github"))) {
    assert.ok(TEXT.some((f) => /^\.github\/workflows\/.+\.ya?ml$/.test(f)), "the walk missed the CI configuration")
  }
})

test("every JSON and YAML text file is LF, with no CR anywhere", () => {
  const config = TEXT.filter((f) => /\.(json|ya?ml)$/.test(f))
  assert.ok(config.length > 0, "the walk found no JSON or YAML files")
  for (const rel of sortedFiles(config)) {
    const cr = read(rel).indexOf("\r")
    assert.equal(cr, -1, `${rel} has a CR; the repo is LF-only`)
  }
})

test("every JS file is LF, with no CR anywhere", () => {
  for (const rel of sortedFiles(JS)) {
    const text = read(rel)
    const cr = text.indexOf("\r")
    assert.equal(cr, -1, `${rel} has a CR at byte ${cr} (line ${text.slice(0, cr).split("\n").length}); the repo is LF-only`)
  }
})

test("every JS file ends with a single newline", () => {
  for (const rel of sortedFiles(JS)) {
    const text = read(rel)
    assert.ok(text.endsWith("\n"), `${rel} does not end with a newline`)
    assert.ok(!text.endsWith("\n\n"), `${rel} ends with more than one newline`)
  }
})

test("no JS file has trailing whitespace on a line", () => {
  for (const rel of sortedFiles(JS)) {
    const lines = read(rel).split("\n")
    lines.forEach((line, i) => {
      assert.ok(!/[ \t]+$/.test(line), `${rel}:${i + 1} has trailing whitespace`)
    })
  }
})

test("no JS file indents with a tab", () => {
  for (const rel of sortedFiles(JS)) {
    const lines = read(rel).split("\n")
    lines.forEach((line, i) => {
      assert.ok(!/^\t/.test(line), `${rel}:${i + 1} indents with a tab; the style is spaces`)
    })
  }
})

test("every indented JS line is a multiple of two spaces", () => {
  // The floor is the style .editorconfig declares. A line that opens with an
  // odd number of spaces is a mis-indent an editor made, not a deliberate
  // alignment (continuations still land on an even column).
  for (const rel of sortedFiles(JS)) {
    const lines = read(rel).split("\n")
    lines.forEach((line, i) => {
      const m = /^( +)/.exec(line)
      if (!m) return
      assert.equal(m[1].length % 2, 0, `${rel}:${i + 1} indents by ${m[1].length} spaces, not a multiple of two`)
    })
  }
})

test("the markdown files are LF and end with one newline", () => {
  for (const rel of sortedFiles(MD)) {
    const text = read(rel)
    assert.equal(text.indexOf("\r"), -1, `${rel} has a CR; the repo is LF-only`)
    assert.ok(text.endsWith("\n") && !text.endsWith("\n\n"), `${rel} must end with exactly one newline`)
  }
})
