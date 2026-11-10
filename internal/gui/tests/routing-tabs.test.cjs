// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 拆分成多个 tab，避免滚动很久. The Routing page's requests,
// groups and history sit one below another; a tab above them brings each part
// to the top of the view — Live, Groups, Pools, History — and a part there
// isn't has no tab. The page itself is unchanged. English and Chinese,
// Chromium and WebKit; no backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const models = [
  { id: "a/m", name: "m", providerName: "A", icon: "generic" },
  { id: "b/m", name: "m", providerName: "B", icon: "generic" },
];
const info = (ids) => ids.map((id) => ({ id, ready: true }));
const found = { id: "auto-m", name: "Model M", members: ["a/m", "b/m"], auto: true, ready: true, memberInfo: info(["a/m", "b/m"]) };
const now = Date.now();
const routes = Array.from({ length: 60 }, (_, i) => ({
  id: "r" + i, seq: i + 1, at: new Date(now - i * 60000).toISOString(), agent: "claude", asked: "group/auto-m",
  model: "a/m", provider: "a", status: 200, ms: 900, ttft: 300, in: 100, out: 20, done: true,
}));
const L = { en: { live: "Live", groups: "Groups", history: "History" }, zh: { live: "实时", groups: "分组", history: "历史" } };

function serve(lang) {
  const state = { agents: [{ id: "claude", name: "Claude Code", path: "/test/claude", fields: [] }], profiles: [], settings: { lang, theme: "light" } };
  return async (r) => {
    const url = new URL(r.request().url());
    const json = (data) => r.fulfill({ json: data });
    if (url.pathname === "/boot.js") return r.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return r.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/gateway/trace") {
      if (url.searchParams.get("wait")) return new Promise(() => {});
      return json({ mine: true, now: new Date().toISOString(), seq: routes.length, totals: { requests: routes.length, rerouted: 0, errors: 0 }, routes });
    }
    if (url.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (url.pathname === "/api/groups") return json({ models, pools: [], deciders: [], found: true, groups: [found] });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true, url: "http://127.0.0.1:3999" } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await r.fulfill({ body: await fs.readFile(file), contentType });
  };
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the Routing page's tabs bring each part to the top`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1000, height: 600 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(15000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang));
      await page.goto("http://magpie.test/?view=routing");
      await page.locator(".rt-group", { hasText: "Model M" }).waitFor({ state: "attached" });
      const tabs = page.locator(".rt-tabs");
      await tabs.waitFor();
      const names = await tabs.locator("button:not([hidden])").allTextContents();
      assert.deepEqual(names.map((x) => x.trim()), [w.live, w.groups], "Live and Groups; no Pools or History without them");
      const view = page.locator("#view-routing");
      const top = () => view.evaluate((v) => v.scrollTop);
      // Groups: the view moves down (the head's own Routing groups does the
      // exact landing; here the tab is wired to the same section)
      const jump = (key) => tabs.locator(`button[data-jump="${key}"]`).click();
      // a click no reader made moves nothing
      await tabs.locator('button[data-jump="groups"]').evaluate((b) => b.click());
      await page.waitForTimeout(200);
      assert.equal(await top(), 0, "an untrusted click doesn't move the page");
      await jump("groups");
      await page.waitForTimeout(400);
      assert((await top()) > 0, "the groups' tab takes the view down");
      // Live: back to the top
      await jump("live");
      await page.waitForTimeout(400);
      assert.equal(await top(), 0);
      assert.deepEqual(errors, []);
      await page.context().close();
    });
  }
}
