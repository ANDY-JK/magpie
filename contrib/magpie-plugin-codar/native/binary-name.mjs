// native/binary-name.mjs: one naming rule for the proxy's builds.
//
// Both sides use it: scripts/build-native.mjs names the files it writes, and
// native.mjs names the file it looks for. They were written apart once and
// disagreed on Windows ("windows" against Node's "win32"), so a Windows build
// existed and was never found. A build is named by the Node platform and
// architecture that runs it — darwin/linux/win32 and arm64/amd64 — whatever
// Go calls them.

const platformName = (platform) => platform
const archName = (arch) => (arch === "x64" ? "amd64" : arch === "ia32" ? "386" : arch)

// binaryFile is the file name of the build for one platform and architecture.
export function binaryFile(platform, arch) {
  return `codar-proxy-${platformName(platform)}-${archName(arch)}${platformName(platform) === "win32" ? ".exe" : ""}`
}

// packageCpu, packageOs and packageName name the npm package one build ships
// in. Each platform's build travels in its own package — five builds in one
// package was ~30 MB of binaries for every install, of which one is ever used —
// and a package's `os`/`cpu` fields are what a package manager matches against
// the machine, so they are spelled the way npm and bun spell them, not the way
// Go does. npm rejects a `cpu` it does not know: `cpu:["amd64"]` is
// EBADPLATFORM ("Actual cpu: x64", npm 12.2.1, measured 2026-11-15) while
// `cpu:["x64"]` installs, and bun does not check the field at all — so the Go
// spelling would install for one package manager and fail for the other.
// `os` is Node's platform name (darwin/linux/win32), which is what both read.
export const packageCpu = (arch) => archName(arch) === "amd64" ? "x64" : archName(arch)
export const packageOs = (platform) => platformName(platform)

// packageName is that package's name, from the same platform and architecture
// binaryFile takes.
export const packageName = (platform, arch) => `magpie-plugin-codar-${packageOs(platform)}-${packageCpu(arch)}`
