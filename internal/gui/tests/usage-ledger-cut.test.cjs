// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): a reply that stopped after two sentences should be told
// from a provider's failure. A request whose upstream stop reason is its
// output limit (OpenAI's length, Anthropic's or Gemini's max_tokens,
// Responses' max_output_tokens) is marked "cut at the limit" in its row, with
// why and what to do on hover, and says the same in its details; a reply that
// ended on its own says nothing. English and Chinese, both engines; no
// backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const now = Date.now();
const WHY = "The vendor stopped the reply at its output limit: the model's own, or this provider's Max output. Raise it there, or continue in a shorter conversation.";
const ROWS = [
  { t: new Date(now - 60e3).toISOString(), agent: "codex", agentName: "Codex", icon: "codex-color", provider: "relay", providerName: "Relay", host: "team", req: "sol", model: "gpt-6-sol", served: "gpt-6-sol", in: 120, out: 64, ms: 900, ttft_ms: 200, status: 200, stop: "length", ep: "/v1/chat/completions", cost: 0, priced: false },
  { t: new Date(now - 120e3).toISOString(), agent: "codex", agentName: "Codex", icon: "codex-color", provider: "relay", providerName: "Relay", host: "team", req: "sol", model: "gpt-6-sol", served: "gpt-6-sol", in: 120, out: 300, ms: 1200, ttft_ms: 220, status: 200, stop: "stop", ep: "/v1/chat/completions", cost: 0, priced: false },
];
const L = {
  en: { cut: "cut at the limit", why: WHY, stop: "length · cut at the limit" },
  zh: { cut: "达到输出上限", why: "厂商在输出上限处结束了回复：可能是模型自身的上限，也可能是本供应商的「最大输出」设置。请在那里调大，或缩短对话后继续。", stop: "length · 达到输出上限" },
};

function page(q) {
  const rows = ROWS;
  return { period: q.get("period"), rows, offset: 0, total: rows.length, calls: rows.length, errors: 0, input: 240, output: 364, cache_read: 0, cache_write: 0, reasoning: 0, cost: 0, unpriced: 2, agents: [{ id: "codex", name: "Codex", icon: "codex-color" }] };
}

function server(lang) {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/usage/requests") return json(page(url.searchParams));
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/usage") return json({ calls: 2, errors: 0, input: 240, output: 364, cache_read: 0, cache_write: 0, reasoning: 0, unpriced: 2, cost: 0, bucket: "day", series: [], agents: [], models: [] });
    if (url.pathname === "/api/sessions") return json({ sessions: [], dirs: [] });
    if (url.pathname === "/api/sessions/stats") return json({ from: "", to: "", days: [], agents: {} });
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

let I18N;
const loadI18N = () => { I18N = strings().I18N; };
function tr(lang, key, vars = {}) {
  let s = key;
  if (lang !== "en") {
    assert.equal(typeof I18N[lang][key], "string", `${lang} has no ${JSON.stringify(key)}`);
    s = I18N[lang][key];
  }
  return s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a reply cut at its output limit is marked", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    loadI18N();
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh"]) {
      await t.test(lang, async () => {
        const w = (key, vars) => tr(lang, key, vars);
        const errors = [];
        const ctx = await browser.newContext({ viewport: { width: 1180, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
        const p = await ctx.newPage();
        p.setDefaultTimeout(5000);
        p.setDefaultNavigationTimeout(60000);
        p.on("pageerror", (e) => errors.push(e.message));
        await p.route("**/*", server(lang));
        await p.goto("http://magpie.test/");
        await p.locator('[data-view="usage"]').first().click();
        await p.locator(":is(#usageTab .opt, .nav-s):visible").nth(1).click();
        await p.locator("#ledWrap .led tbody tr.led-row").first().waitFor();
        const rows = p.locator(".led tbody tr.led-row");
        assert.equal(await rows.count(), 2);
        const cut = rows.nth(0).locator(".st .led-cut");
        assert.equal(await cut.count(), 1, "the cut reply is marked");
        assert.equal((await cut.textContent()).trim(), w("cut at the limit"));
        assert.equal(await cut.getAttribute("title"), w(WHY));
        assert.equal(await rows.nth(1).locator(".led-cut").count(), 0, "a reply that ended on its own is not marked");
        // its details say the same, and that the reason is the limit
        await rows.nth(0).click();
        const detail = p.locator(".led tbody tr.led-detail").first();
        await detail.waitFor();
        const text = await detail.textContent();
        assert(text.includes(w("{stop} · cut at the limit", { stop: "length" })), `the details say the stop reason: ${text}`);
        assert.equal(await rows.nth(0).evaluate((tr) => tr.querySelector(".st").closest("td").title), w(WHY));
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
}
