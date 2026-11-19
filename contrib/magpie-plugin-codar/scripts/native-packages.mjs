// native-packages.mjs: the platform packages the five proxy builds ship in.
//
// One package per build, not five builds in one package. The proxy is a static
// Go binary about 6 MB each, and every install of the old single package
// carried all five — 32.0 MB unpacked (npm 12.2.1, 0.2.4) — while one of them
// is ever run. Each platform's build now travels in its own package —
// magpie-plugin-codar-<os>-<cpu>, with os and cpu spelled the way npm and bun
// match them (see native/binary-name.mjs) — and the main package names all
// five in optionalDependencies, so a package manager installs the one that
// fits the machine and skips the rest. Measured 2026-11-15 on bun 1.4.4 and
// npm 12.2.1: from a tarball or from the registry, each installs exactly the
// matching package (bun before this was checked with @esbuild's own platform
// packages). Installing one here cannot be tested against the main package:
// a `file:` optionalDependencies entry is not filtered by platform at all, and
// bun installs every one of them.
//
// Nothing runs at install time: the main package finds the build by resolving
// the package it was given (lib/native.mjs binaryPath). That is the only shape
// that works for both package managers — bun blocks postinstall scripts of
// packages it does not trust (measured 2026-11-15), so a package that fetched
// or placed its build in an install script would silently have no proxy.
//
// This module is that packaging as functions: stage a platform package per
// target, check what has been staged, and say what the main package's manifest
// must carry for the packages to be reached.

import { spawnSync } from "node:child_process"
import { chmodSync, copyFileSync, existsSync, mkdirSync, readFileSync, realpathSync, statSync, writeFileSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

import { binaryFile } from "../native/binary-name.mjs"
import { buildTargets } from "./build-native.mjs"

// npm spells a platform with the Node platform name and an architecture of
// x64/arm64 — never Go's amd64. npm refuses a cpu it does not recognize
// (`cpu:["amd64"]` is EBADPLATFORM, npm 12.2.1, measured 2026-11-15) while bun
// ignores the field, so the Go spelling installs for one package manager and
// fails for the other. See native/binary-name.mjs, which the runtime lookup
// uses to find these packages by the same names.
const packageCpu = (arch) => (arch === "amd64" ? "x64" : arch)
const packageName = (os, arch) => `magpie-plugin-codar-${os}-${packageCpu(arch)}`

// Every target, in the order scripts/build-native.mjs builds them: the build
// table is the one authority for what exists, so this package layout cannot
// list a platform the build does not make. build-native.mjs must not import
// this module back — the table flows one way.
export const platformPackages = (targets = buildTargets) =>
  targets.map((t) => ({
    name: packageName(t.os, t.arch),
    os: t.os,
    cpu: packageCpu(t.arch),
    file: binaryFile(t.os, t.arch),
  }))

const readManifest = (path) => {
  try {
    return JSON.parse(readFileSync(path, "utf8"))
  } catch {
    return null
  }
}

// stagePackages writes one package per target into out, each holding its build
// at bin/<file> with the executable bit an npm tarball will keep (a build that
// arrives without it cannot be spawned, which is how the 0.2.1 tarball failed
// its own gate), and answers what it wrote. A build missing from bin is an
// error: a package staged without one would install and then have no proxy.
export function stagePackages({ bin, out, version, targets = buildTargets }) {
  const staged = []
  for (const p of platformPackages(targets)) {
    const source = join(bin, p.file)
    if (!existsSync(source) || statSync(source).size < 4096) throw new Error(`${p.name}: no build at ${source}; run npm run native:all first`)
    const dir = join(out, p.name)
    const dest = join(dir, "bin", p.file)
    mkdirSync(join(dir, "bin"), { recursive: true })
    copyFileSync(source, dest)
    chmodSync(dest, 0o755)
    const manifest = {
      name: p.name,
      version,
      description: `The Codar desktop transport for magpie-plugin-codar on ${p.os}/${p.cpu}. Install magpie-plugin-codar; this package is chosen for you.`,
      license: "MIT",
      os: [p.os],
      cpu: [p.cpu],
      files: ["bin"],
    }
    writeFileSync(join(dir, "package.json"), JSON.stringify(manifest, null, 2) + "\n")
    staged.push({ ...p, dir })
  }
  return staged
}

// checkPackages reports each staged package as "present" or "broken" with a
// reason: the manifest names the package and the release's version, its os and
// cpu are npm's spelling of the target, and a non-empty executable build is
// there. The version is the argument rather than read off the main package, so
// a caller checking a different version sees every package drift at once.
export function checkPackages({ dir, version, targets = buildTargets }) {
  return platformPackages(targets).map((p) => {
    const where = join(dir, p.name)
    const manifest = readManifest(join(where, "package.json"))
    if (!manifest) return { name: p.name, state: "broken", reason: "no package.json" }
    if (manifest.name !== p.name) return { name: p.name, state: "broken", reason: `its manifest names ${manifest.name}` }
    if (manifest.version !== version) return { name: p.name, state: "broken", reason: `version ${manifest.version}, the release is ${version}` }
    if (JSON.stringify(manifest.os) !== JSON.stringify([p.os]) || JSON.stringify(manifest.cpu) !== JSON.stringify([p.cpu])) {
      return { name: p.name, state: "broken", reason: `os/cpu ${JSON.stringify(manifest.os)}/${JSON.stringify(manifest.cpu)}, want ${JSON.stringify([p.os])}/${JSON.stringify([p.cpu])}` }
    }
    let stat
    try {
      stat = statSync(join(where, "bin", p.file))
    } catch {
      return { name: p.name, state: "broken", reason: `bin/${p.file} missing` }
    }
    if (stat.size < 4096) return { name: p.name, state: "broken", reason: `bin/${p.file} only ${stat.size} bytes` }
    if (process.platform !== "win32" && (stat.mode & 0o111) === 0) return { name: p.name, state: "broken", reason: `bin/${p.file} not executable` }
    return { name: p.name, state: "present" }
  })
}

// mainPackageProblems lists what a main-package manifest gets wrong about the
// platform packages, in words: every package named, each at exactly the
// release's version (a range lets the two drift apart, and a package at
// another version is one nobody asked for), and none of the builds in the
// package itself — five builds shipped twice is the size this replaced.
export function mainPackageProblems(manifest, targets = buildTargets) {
  const problems = []
  const optional = manifest.optionalDependencies ?? {}
  for (const p of platformPackages(targets)) {
    const at = optional[p.name]
    if (at === undefined) problems.push(`${p.name} is not in optionalDependencies`)
    else if (at !== manifest.version) problems.push(`${p.name} is pinned to ${at}; an exact ${manifest.version} is what installs with the main package`)
  }
  const groups = manifest.bundleDependencies ?? manifest.bundledDependencies
  if (Array.isArray(groups) && groups.some((g) => platformPackages(targets).some((p) => p.name === g))) {
    problems.push("a platform package is bundled into the main package")
  }
  const files = manifest.files ?? []
  if (files.some((f) => String(f) === "native/bin")) {
    problems.push("files names native/bin: the builds are the platform packages' now, and five shipped twice is the size this replaced")
  }
  return problems
}

// ---- the command a release runs --------------------------------------------

const here = dirname(fileURLToPath(import.meta.url))
const pkgRoot = join(here, "..")
// staged is where a release writes the five packages before publishing them.
// Not `files`, not committed: a release stages it, publishes it, and leaves it
// (git ignores native/packages, as it ignores native/bin).
const staged = join(pkgRoot, "native", "packages")

// stageFromTree stages every package at the version the main package declares,
// and refuses when the main package's own manifest would not reach them.
export function stageFromTree({ out = staged } = {}) {
  const manifest = JSON.parse(readFileSync(join(pkgRoot, "package.json"), "utf8"))
  const problems = mainPackageProblems(manifest)
  if (problems.length) throw new Error(`package.json: ${problems.join("; ")}`)
  return stagePackages({ bin: join(pkgRoot, "native", "bin"), out, version: manifest.version })
}

// The command:
//   node scripts/native-packages.mjs            stage the five packages
//   node scripts/native-packages.mjs --check    report what has been staged
//                                               (exit 1 unless all five are
//                                               staged at the main version)
//   node scripts/native-packages.mjs --publish  stage, then publish each with
//                                               npm publish --access public
// A release publishes the platform packages before the main package: their
// exact versions are what its optionalDependencies name, and npm refuses a
// dependency it cannot find.
function main(argv) {
  const args = new Set(argv)
  const unknown = [...args].filter((a) => a !== "--check" && a !== "--publish")
  if (unknown.length) {
    console.error(`native-packages: unrecognized argument ${unknown.join(" ")} (try --check or --publish)`)
    return 2
  }
  const manifest = JSON.parse(readFileSync(join(pkgRoot, "package.json"), "utf8"))
  if (args.has("--check")) {
    const rows = checkPackages({ dir: staged, version: manifest.version })
    for (const r of rows) console.log(r.state === "present" ? `present ${r.name}@${manifest.version}` : `broken ${r.name}: ${r.reason}`)
    return rows.some((r) => r.state !== "present") ? 1 : 0
  }
  const wrote = stageFromTree()
  for (const p of wrote) console.log(`staged ${p.name}@${manifest.version} -> ${p.dir}`)
  if (!args.has("--publish")) return 0
  const rows = checkPackages({ dir: staged, version: manifest.version })
  const bad = rows.filter((r) => r.state !== "present")
  if (bad.length) {
    console.error(`native-packages: ${bad.map((r) => `${r.name} ${r.reason}`).join("; ")}`)
    return 1
  }
  for (const p of wrote) {
    // --access public: a scoped name defaults to private, and these are
    // unscoped, but the flag is what npm documents for a first publish.
    const out = spawnSync("npm", ["publish", "--access", "public"], { cwd: p.dir, stdio: "inherit" })
    if (out.status !== 0) {
      console.error(`native-packages: publishing ${p.name} failed (exit ${out.status})`)
      return 1
    }
  }
  return 0
}

const isMain = (() => {
  try {
    return realpathSync(process.argv[1] ?? "") === realpathSync(fileURLToPath(import.meta.url))
  } catch {
    return false
  }
})()

if (isMain) process.exitCode = main(process.argv.slice(2))
