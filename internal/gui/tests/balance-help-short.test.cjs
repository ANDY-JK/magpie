// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 描述看不懂、臃肿. The Balance field's help was one
// 485-character sentence of rules, examples and edge cases; it is now a
// short line for each: what a path is, examples, several labelled amounts,
// lists, $ and %, and an amount the reply hasn't. Every line stays short
// enough to read without hunting. English and Chinese, both engines.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const relay = {
  id: "relay", name: "Relay", icon: "generic", chat: "https://relay.example.com/v1", responses: "", anthropic: "", catalog: "",
  models: [{ id: "model-a", name: "Model A", on: true }], agents: [], fallback: [], headers: {},
  key: { set: true, masked: "sk-…one" }, keyList: [],
  balanceURL: "https://relay.example.com/api/usage/token", balancePath: "",
  balanceToken: { takes: true, set: false },
};

function server(lang) {
  const providers = { providers: [relay], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const L = {
  en: { summary: "Where the amount is in the reply, e.g. data.balance", old: "a list's items are" },
  zh: { summary: "金额在返回 JSON 中的位置，例如 data.balance", old: "陣列的每一項寫作" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the Balance field's help is a line a rule`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 900, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang));
      await page.goto("http://magpie.test/?view=providers");
      await page.locator(".row.provider", { hasText: "Relay" }).click();
      const help = page.locator(".editor .bal-help");
      await help.waitFor({ state: "attached" });
      const lines = (await help.locator("p").allTextContents()).map((x) => x.trim());
      assert.equal(lines.length, 6, "one line a rule");
      assert(lines.every((x) => x.length > 0 && x.length <= 140), "every line stays short: " + JSON.stringify(lines.map((x) => x.length)));
      const all = lines.join("\n");
      for (const part of ["data.total", "sum()", "—"]) assert(all.includes(part), `the help says ${part}: ${all}`);
      // the one long sentence is gone
      assert.equal(await page.locator(".editor", { hasText: w.old }).count(), 0, "the 485-character sentence is gone");
      const summary = await help.evaluate((e) => e.closest("details")?.querySelector("summary")?.textContent.trim() || "");
      assert.equal(summary, w.summary, "the help fold still says what it is");
      assert.deepEqual(errors, []);
      await page.context().close();
    });
  }
}
