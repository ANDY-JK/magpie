// 新进程卸载独立性验证模板。
//
// 用法示例：
//   node verify-fresh-process.mjs \
//     --auth "$HOME/.config/magpie/plugin-auth.json" \
//     --provider dumate \
//     --token-url "https://console.example.com/api/token/get" \
//     --chat-url "https://api.example.com/v1/chat/completions" \
//     --model deepseek-v41-flash
//
// 说明：
// - 只读取 magpie 已保存的登录态（plugin-auth.json），不读桌面应用文件；
// - 每次运行都是新进程，不复用任何 token 缓存；
// - 只打印阶段、HTTP 状态、token 长度、模型回显，不打印 Cookie/token；
// - 拒绝把 127.0.0.1/localhost 当云端地址。

import fs from "node:fs"

const arg = (name, fallback = "") => {
  const i = process.argv.indexOf(`--${name}`)
  return i > 0 && i + 1 < process.argv.length ? process.argv[i + 1] : fallback
}

const authPath = arg("auth")
const provider = arg("provider")
const tokenUrl = arg("token-url")
const chatUrl = arg("chat-url")
const model = arg("model")
const tokenPath = arg("token-path", "result.token")

const die = (stage, message) => {
  console.log(`FAIL stage=${stage}: ${message}`)
  process.exit(1)
}

if (!authPath || !provider || !chatUrl || !model) {
  die("args", "need --auth --provider --chat-url --model")
}
if (/^https?:\/\/(?:127\.0\.0\.1|localhost|\[::1\])(?::\d+)?/i.test(chatUrl)) {
  die("args", "chat URL is a local proxy; use the real cloud endpoint, not 127.0.0.1/localhost")
}

const auth = JSON.parse(fs.readFileSync(authPath, "utf8"))?.[provider]
if (!auth) die("auth", `no "${provider}" in plugin-auth.json`)
if (!Array.isArray(auth.cookies) && !auth.token && !auth.access) {
  die("auth", "saved sign-in has no cookies/token/access")
}

const headers = { Accept: "application/json" }
if (Array.isArray(auth.cookies) && auth.cookies.length) {
  headers.Cookie = auth.cookies.map((c) => `${c.name}=${c.value}`).join("; ")
} else {
  headers.Authorization = `Bearer ${String(auth.token ?? auth.access).replace(/^Bearer\s+/i, "")}`
}

let token = String(auth.token ?? auth.access ?? "").replace(/^Bearer\s+/i, "")

if (tokenUrl) {
  const res = await fetch(tokenUrl, { headers, signal: AbortSignal.timeout(25000) })
  const text = await res.text()
  let json = null
  try { json = JSON.parse(text) } catch {}
  console.log(`stage=token_exchange status=${res.status} json=${!!json}`)
  if (!res.ok) die("token_exchange", `HTTP ${res.status}`)
  if (!json) die("token_exchange", "non-JSON response (login page?)")
  token = String(tokenPath.split(".").reduce((o, k) => o?.[k], json) ?? "")
  if (!token) die("token_exchange", "no token in response")
  console.log(`stage=token_exchange tokenLen=${token.length}`)
}

const res = await fetch(chatUrl, {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    Authorization: `Bearer ${token}`,
  },
  body: JSON.stringify({
    model,
    messages: [{ role: "user", content: "hi" }],
    max_tokens: 1,
    stream: false,
  }),
  signal: AbortSignal.timeout(40000),
})

const text = await res.text()
console.log(`stage=cloud_chat status=${res.status} endpoint=${new URL(chatUrl).host}`)
if (!res.ok) {
  console.log("response:", text.slice(0, 400).replace(/\s+/g, " "))
  process.exit(1)
}
let out = null
try { out = JSON.parse(text) } catch {}
console.log("model echoed:", out?.model ?? "(non-JSON)")
console.log("completion_tokens:", out?.usage?.completion_tokens ?? "(unknown)")
console.log("OK: fresh process exchanged a token and called the cloud endpoint directly")
