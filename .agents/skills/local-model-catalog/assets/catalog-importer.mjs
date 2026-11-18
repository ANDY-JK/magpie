// 独立模型清单模板：复制到插件目录，按 CONFIG 改成本应用的值。
//
// 设计目标：
// - 源配置存在时重新导入；
// - 源缺失/损坏时保留最近一次有效清单；
// - 用户手动模型与覆盖放 custom，导入时保留；
// - models.json 只含模型元数据，不含凭据、账号 ID、端口、用户名。

import fs from "node:fs"
import os from "node:os"
import path from "node:path"
import { fileURLToPath } from "node:url"

// 按目标应用修改这一块。
export const CONFIG = {
  catalogFile: path.join(path.dirname(fileURLToPath(import.meta.url)), "models.json"),
  // 显式路径优先；最后一个可直接指向模型配置文件。
  env: {
    data: ["APP_USER_DATA_DIR"],
    xdg: ["APP_XDG_DIR"],
    config: ["APP_OPENCODE_JSON", "APP_MODEL_CONFIG"],
    configRoot: ["APP_CONFIG_DIR"],
  },
  appNames: ["AppName"],
  // 数据根下要尝试的子目录；desktop-app 很常见。
  dataChildren: ["", "desktop-app"],
  // 账号配置树的名字（例如 qianfan_desk_xdg）；没有就留空。
  xdgName: "",
  configTail: ["config", "opencode", "opencode.json"],
  // 应用自己的“当前账号”文件，常见字段：ownerDir / scope / accountId。
  ownerFiles: ["current-owner.json"],
  // 账号配置不在 configTail 时，提供两个函数之一：
  // accountConfig: (root, account) => path.join(root, "users", account, "App.jsonc")
  // globalConfig:  (root) => path.join(root, "global", "config", "opencode", "opencode.json")
  accountConfig: null,
  globalConfig: null,
  // 模型所在的 provider 键；按顺序找第一个存在的。
  providerKeys: ["ProviderName"],
  // 明确排除的内部入口；正则兜底。
  excludeKeys: new Set([
    "llm-mock", "qianfan-ocr", "qwen3-embedding-4b", "qwen3-reranker-4b", "dumate-image0.1",
    "model-aiassist", "model-artifact-validate", "model-coding-routing",
    "model-followup", "model-gui-use", "model-meeting-notes", "model-mm-fallback",
    "model-ppt-validate", "model-reason-summary", "model-routing", "model-routing2",
    "model-tm-deepseek", "model-tm-qwen-small", "model-wiki", "tele-safety-answer",
  ]),
  excludeRe: /(?:^|[-_])(?:mock|ocr|embedding|reranker|rerank|validate|safety)(?:$|[-_])/i,
  // 从未成功导入时的最小兜底；按应用改。
  fallback: [
    { id: "model-text", name: "Default", context: 192000, output: 128000 },
  ],
}

const num = (v) => (Number.isFinite(Number(v)) && Number(v) > 0 ? Math.round(Number(v)) : 0)
const strings = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === "string" && x).map(String) : [])
const envDir = (names) => {
  for (const n of names) {
    const v = String(process.env[n] ?? "").trim()
    if (v) return v
  }
  return ""
}

// ---- 路径 ------------------------------------------------------------------

function osDefaults() {
  const home = os.homedir()
  const roots = []
  if (process.platform === "win32") {
    roots.push(process.env.APPDATA || path.join(home, "AppData", "Roaming"))
    roots.push(process.env.LOCALAPPDATA || path.join(home, "AppData", "Local"))
    roots.push(path.join(home, ".config"), path.join(home, ".local", "share"))
  } else if (process.platform === "darwin") {
    roots.push(path.join(home, "Library", "Application Support"))
  } else {
    roots.push(process.env.XDG_CONFIG_HOME || path.join(home, ".config"))
    roots.push(process.env.XDG_DATA_HOME || path.join(home, ".local", "share"))
  }
  return roots
}

function installDataDirs() {
  const out = []
  for (const root of osDefaults()) {
    for (const name of CONFIG.appNames) {
      for (const f of ["install.json", `${name.toLowerCase()}-install.json`]) {
        try {
          const j = JSON.parse(fs.readFileSync(path.join(root, name, f), "utf8"))
          const d = String(j?.dataDir ?? "").trim()
          if (d && !out.includes(d)) out.push(d)
        } catch {}
      }
    }
  }
  return out
}

export function dataDirs() {
  const explicit = envDir(CONFIG.env.data)
  const bases = explicit
    ? [explicit]
    : [...installDataDirs(), ...osDefaults().flatMap((r) => CONFIG.appNames.map((n) => path.join(r, n)))]
  const out = []
  for (const b of bases) for (const c of CONFIG.dataChildren) {
    const d = c ? path.join(b, c) : b
    if (!out.includes(d)) out.push(d)
  }
  return out
}

function osConfigRoots() {
  const home = os.homedir()
  if (process.platform === "win32") {
    return [
      process.env.APPDATA || path.join(home, "AppData", "Roaming"),
      path.join(home, ".config"),
    ]
  }
  if (process.platform === "darwin") {
    return [path.join(home, "Library", "Application Support"), path.join(home, ".config")]
  }
  return [process.env.XDG_CONFIG_HOME || path.join(home, ".config")]
}

// 配置根不一定等于数据根：有的应用数据在 .local/share，配置在 .config。
export function configRoots() {
  const explicit = envDir(CONFIG.env.configRoot)
  if (explicit) return [explicit]
  const out = []
  for (const root of osConfigRoots()) for (const n of CONFIG.appNames) out.push(path.join(root, n))
  for (const d of dataDirs()) out.push(d)
  return [...new Set(out)]
}

function xdgCandidates() {
  const explicit = envDir(CONFIG.env.xdg)
  if (explicit) return [explicit]
  const out = []
  for (const d of configRoots()) {
    if (CONFIG.xdgName) out.push(path.join(d, CONFIG.xdgName))
    out.push(d)
  }
  return [...new Set(out)]
}

// 当前账号目录：先看应用自己的 owner 文件，再看 auth.json 的 profile 映射。
export function activeAccountDirs() {
  const out = []
  const push = (v) => {
    const s = String(v ?? "").trim()
    if (s && !out.includes(s)) out.push(s)
  }
  for (const d of [...dataDirs(), ...configRoots()]) {
    for (const f of CONFIG.ownerFiles ?? []) {
      try {
        const j = JSON.parse(fs.readFileSync(path.join(d, f), "utf8"))
        for (const v of [j?.ownerDir, j?.scope, j?.accountId]) push(v)
      } catch {}
    }
  }
  if (out.length) return out
  for (const d of [...dataDirs(), ...configRoots()]) {
    try {
      const auth = JSON.parse(fs.readFileSync(path.join(d, "auth.json"), "utf8"))
      const id = String(auth?.activeProfileId ?? "").trim()
      const profiles = Array.isArray(auth?.accountProfiles) ? auth.accountProfiles : []
      const p = profiles.find((x) => String(x?.profileId ?? "") === id) ?? profiles[0] ?? null
      for (const v of [p?.bceAccountId, p?.profileId, id, p?.userId]) push(v)
      if (out.length) return out
    } catch {}
  }
  return out
}

function accountConfig(root, account) {
  if (typeof CONFIG.accountConfig === "function") return CONFIG.accountConfig(root, account)
  return path.join(root, account, ...CONFIG.configTail)
}

function globalConfig(root) {
  if (typeof CONFIG.globalConfig === "function") return CONFIG.globalConfig(root)
  return path.join(root, "global", ...CONFIG.configTail)
}

export function sourceConfig() {
  const explicit = envDir(CONFIG.env.config)
  if (explicit) return explicit
  const accounts = activeAccountDirs()
  const candidates = []
  for (const root of xdgCandidates()) {
    for (const a of accounts) candidates.push(accountConfig(root, a))
    candidates.push(globalConfig(root))
    try {
      for (const e of fs.readdirSync(root, { withFileTypes: true })) {
        if (!e.isDirectory() || ["data", "global", "cache", "users"].includes(e.name)) continue
        const p = accountConfig(root, e.name)
        if (!candidates.includes(p)) candidates.push(p)
      }
    } catch {}
  }
  for (const p of candidates) {
    try {
      const st = fs.statSync(p)
      if (st.isFile() && st.size > 0 && st.size < 4 * 1024 * 1024) return p
    } catch {}
  }
  return ""
}

// ---- JSONC -----------------------------------------------------------------

export function stripJsonc(text) {
  let out = "", inStr = false, esc = false, line = false, block = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i], n = text[i + 1]
    if (line) { if (c === "\n") { line = false; out += c } continue }
    if (block) { if (c === "*" && n === "/") { block = false; i++ } continue }
    if (inStr) {
      out += c
      if (esc) esc = false
      else if (c === "\\") esc = true
      else if (c === '"') inStr = false
      continue
    }
    if (c === '"') { inStr = true; out += c; continue }
    if (c === "/" && n === "/") { line = true; i++; continue }
    if (c === "/" && n === "*") { block = true; i++; continue }
    out += c
  }
  return out
}

export function readJsonLoose(file) {
  return JSON.parse(stripJsonc(fs.readFileSync(file, "utf8")).replace(/,\s*([}\]])/g, "$1"))
}

// ---- 提取 ------------------------------------------------------------------

function cleanModel(key, raw) {
  const apiId = String(raw?.id ?? key).trim()
  if (!apiId || apiId.length > 200) return null
  const input = strings(raw?.modalities?.input)
  const output = strings(raw?.modalities?.output)
  const mode = String(raw?.thinking?.mode ?? "").toLowerCase()
  return {
    apiId,
    key,
    model: {
      id: apiId,
      name: String(raw?.name ?? key).trim() || key,
      limit: {
        context: num(raw?.limit?.context ?? raw?.limit?.input),
        output: num(raw?.limit?.output),
      },
      modalities: {
        input: input.length ? input : ["text"],
        output: output.length ? output : ["text"],
      },
      // 源里明确说不支持时才关；大多数 OpenAI 兼容聊天模型支持 tools
      tool_call: raw?.tool_call !== false && raw?.tools !== false,
      // 没有显式依据就 false，别凭名字猜
      reasoning: raw?.reasoning === true || raw?.thinking?.enabled === true ||
        (mode !== "" && mode !== "never" && mode !== "off"),
      attachment: input.some((m) => ["image", "video", "audio"].includes(m)),
    },
  }
}

export function extractModels(doc) {
  const providers = doc?.provider
  if (!providers || typeof providers !== "object") throw new Error("no provider section")
  const entry = CONFIG.providerKeys.map((k) => providers[k]).find((p) => p && typeof p === "object" && p.models)
    ?? Object.values(providers).find((p) => p && typeof p === "object" && p.models)
  const source = entry?.models
  if (!source || typeof source !== "object") throw new Error("no models in provider config")
  const rows = Array.isArray(source)
    ? source.map((m, i) => [String(m?.id ?? i), m])
    : Object.entries(source)
  const byId = new Map()
  for (const [key, raw] of rows) {
    if (typeof key !== "string" || !key || CONFIG.excludeKeys.has(key) || CONFIG.excludeRe.test(key)) continue
    if (!raw || typeof raw !== "object") continue
    const got = cleanModel(key, raw)
    if (!got || CONFIG.excludeRe.test(got.apiId)) continue
    const prev = byId.get(got.apiId)
    if (!prev || (key === got.apiId && prev.key !== got.apiId)) byId.set(got.apiId, got)
  }
  if (!byId.size) throw new Error("no chat models after filtering")
  const models = {}
  for (const [id, { model }] of byId) models[id] = model
  return models
}

// ---- 清单 ------------------------------------------------------------------

function fallbackModels() {
  const out = {}
  for (const m of CONFIG.fallback) {
    out[m.id] = {
      id: m.id,
      name: m.name,
      limit: { context: m.context, output: m.output },
      modalities: { input: ["text"], output: ["text"] },
      tool_call: true,
      reasoning: false,
      attachment: false,
    }
  }
  return out
}

function loadCatalog() {
  try {
    const c = JSON.parse(fs.readFileSync(CONFIG.catalogFile, "utf8"))
    if (!c?.models || typeof c.models !== "object") throw new Error("bad shape")
    return {
      models: c.models,
      custom: c.custom && typeof c.custom === "object" ? c.custom : {},
      importedAt: c.importedAt ?? "",
    }
  } catch {
    return { models: fallbackModels(), custom: {}, importedAt: "" }
  }
}

function saveCatalog(c) {
  const tmp = CONFIG.catalogFile + ".tmp"
  fs.writeFileSync(tmp, JSON.stringify(c, null, 2) + "\n", "utf8")
  fs.renameSync(tmp, CONFIG.catalogFile)
}

// 每次 config 钩子调用；userModels 是用户写在 magpie 配置里的模型/覆盖。
export function readModelsConfig(userModels = {}) {
  const saved = loadCatalog()
  const custom = { ...saved.custom }
  for (const [key, m] of Object.entries(userModels ?? {})) {
    if (m && typeof m === "object" && !(key in saved.models) && !(key in custom)) custom[key] = m
  }
  let models = saved.models
  let imported = false
  const source = sourceConfig()
  if (source) {
    try {
      models = extractModels(readJsonLoose(source))
      imported = true
    } catch {
      // 源损坏不丢已有清单
    }
  }
  if (imported) {
    try {
      saveCatalog({
        version: 1,
        importedAt: new Date().toISOString(),
        sourceKind: `${CONFIG.providerKeys[0]}.models`,
        models,
        custom,
      })
    } catch {}
  }
  const defs = {}
  for (const [key, m] of Object.entries({ ...models, ...custom })) {
    defs[key] = {
      ...m,
      id: m?.id ?? key,
      name: m?.name ?? key,
      limit: { ...(m?.limit ?? {}) },
      modalities: { ...(m?.modalities ?? {}) },
    }
  }
  return { defs, models, custom, imported, source, count: Object.keys(models).length }
}
