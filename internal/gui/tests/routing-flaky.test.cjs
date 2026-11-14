// Run with Node's test runner and Playwright on the module path; see README.md.
// gateway/health.go: a model whose answers have been intermittent is tried
// after the steady ones of its plan, and the trace's Weighed carries the
// score (Weighed.Health). The Routing page says so on the row — "flaky
// lately · tried after the steady ones" — so the late try reads as the
// model's, not the account's; a steady row says nothing of it. English and
// Chinese, Chromium and WebKit; the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const now = Date.now();
const iso = (ms) => new Date(ms).toISOString();
const accts = [
  { id: "acct-1", provider: "codex", name: "Codex", who: "flaky@example.com", kind: "account", model: "gpt-6", known: true, used: 30, plan: "Plus", health: 0.34 },
  { id: "acct-2", provider: "codex", name: "Codex", who: "steady@example.com", kind: "account", model: "gpt-6", known: true, used: 30, plan: "Pro" },
];
const route = {
  id: 7, seq: 7, time: iso(now - 2000), agent: "codex", model: "gpt-6", provider: "codex", done: true, status: 200, ms: 900, tokens: 1500,
  order: accts, tries: [{ id: "acct-2", model: "gpt-6", start: iso(now - 2000), done: true, ms: 800, status: 200 }],
};
const words = {
  en: { flaky: "flaky lately · tried after the steady ones" },
  zh: { flaky: "最近不稳定 · 排在稳定模型之后" },
};

function serve(lang) {
  const state = { agents: [{ id: "codex", name: "Codex", path: "/test/codex", fields: [] }], profiles: [], settings: { lang, theme: "light" } };
  return async (r) => {
    const url = new URL(r.request().url());
    const json = (data) => r.fulfill({ json: data });
    if (url.pathname === "/boot.js") return r.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return r.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/gateway/trace") {
      if (url.searchParams.get("wait")) return new Promise(() => {});
      return json({ mine: true, now: new Date().toISOString(), seq: 7, totals: { requests: 1, rerouted: 0, errors: 0 }, routes: [route] });
    }
    if (url.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (url.pathname === "/api/groups") return json({ models: [], groups: [], pools: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], excluded: [], gateway: { running: true, window: true, url: "http://127.0.0.1:3999" } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await r.fulfill({ body: await fs.readFile(file), contentType });
  };
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of Object.keys(words)) {
    const w = words[lang];
    test(`${engine} ${lang}: a flaky member's row says it is tried after the steady ones`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1100, height: 1200 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang));
      await page.goto("http://magpie.test/?view=routing");
      // the stage's rows say who they are by their who line
      const stage = (who) => page.locator("li", { has: page.locator(".who", { hasText: who }) }).first();
      const flaky = stage("flaky@example.com");
      await flaky.waitFor();
      const said = (await flaky.locator("em").textContent()).trim();
      assert.ok(said.includes(w.flaky), `the flaky row says it: ${said}`);
      const steady = stage("steady@example.com");
      const quiet = (await steady.locator("em").textContent()).trim();
      assert.ok(!quiet.includes(w.flaky.split(" · ")[0]), `the steady row says nothing of it: ${quiet}`);
      assert.deepEqual(errors, []);
      await browser.close();
    });
  }
}
