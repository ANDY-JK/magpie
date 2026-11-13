// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): a provider's model says what is known of it besides its
// name — 上下文、最大输出、压缩阈值、图片、推理档位 — in the hover of its
// chip in the provider editor, one line each. The window and the reply limit
// are the ones agents are told (the user's setting over its vendor's); a
// model with no threshold of its own compacts where Long conversations says
// (here none, so the line says it follows that setting); a provider's own
// threshold is said as the size. Every language; no backend: the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const LANG_TAGS = { en: "en-US", zh: "zh-CN", "zh-TW": "zh-TW", ja: "ja-JP", de: "de-DE" };
const model = (id, more = {}) => ({ id, name: id, on: true, context: 200000, output: 64000, images: true, efforts: ["low", "high"], ...more });
const base = {
  icon: "generic", chat: "http://127.0.0.1:8317/v1", responses: "", anthropic: "", catalog: "",
  models: [], agents: [], fallback: [], headers: {}, balanceToken: { takes: false, set: false }, proxy: "", searches: false,
  key: { set: true, masked: "sk-…aaa" }, keyList: [],
};
const own = { ...base, id: "own", name: "Own Threshold", maxConcurrency: 4, models: [model("glm-5")], compacts: { "glm-5": 128000 } };
const follows = { ...base, id: "follows", name: "Follows Long conversations", models: [model("glm-5")] };
// the concurrency in force is the plugin's when the user set none
// (provider.AtOnce, laneLimitOf); a key or account may hold its own
const pluginLim = { ...base, id: "plim", name: "Plugin Limit", pluginConcurrency: 2, accountConcurrency: { "u-1": 1 }, models: [model("glm-5")] };
// a window the user typed compacts whole (provider.CompactFor, #1809)
const typed = { ...base, id: "typed", name: "Typed Window", models: [model("glm-5", { context: 500000, listed: 200000 })] };
// the price as Usage counts it, what it takes and gives, its release day
const priced = {
  ...base, id: "priced", name: "Priced",
  models: [
    model("glm-5", { price: { input: 2, output: 8, cache_read: 0.2, cache_write: 0.4 }, inputs: ["text", "image"], outputs: ["text"], released: "2026-11-01" }),
    model("air-5", { list: { input: 0.5, output: 2 } }),
  ],
};

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

function serve(lang, extra = {}) {
  const providers = { providers: [own, follows, pluginLim, typed, priced], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light", compactAt: 0, ...extra } };
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

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a model's facts are in its chip's hover", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    loadI18N();
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
      await t.test(lang, async () => {
        const w = (key, vars) => tr(lang, key, vars);
        const num = (n) => n.toLocaleString(LANG_TAGS[lang]);
        const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 }, reducedMotion: "reduce", locale: LANG_TAGS[lang] });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="providers"]').click();
        const row = (name) => page.locator("#providers .row.provider").filter({ has: page.locator(".name", { hasText: name }) }).first();
        const facts = async (provider) => {
          await row(provider).locator(".name").click();
          const ed = page.locator("#modal .editor");
          await ed.waitFor();
          const chip = ed.locator(".mchip.pick", { hasText: "glm-5" }).first();
          await chip.waitFor();
          const title = await chip.getAttribute("title");
          await ed.getByRole("button", { name: w("Cancel"), exact: true }).click();
          await ed.waitFor({ state: "detached" });
          return title;
        };
        // what is known of it, one line each
        const title = await facts("Own Threshold");
        assert(title.includes(w("Context: {n} tokens", { n: num(200000) })), `context: ${title}`);
        assert(title.includes(w("Output: up to {n} tokens", { n: num(64000) })), `output: ${title}`);
        assert(title.includes(w("Compact at {n}", { n: "128K" })), `its own threshold: ${title}`);
        assert(title.includes(w("Concurrency: {n} at once", { n: 4 })), `concurrency: ${title}`);
        assert(title.includes(w("Accepts images")), `images: ${title}`);
        assert(title.includes(w("Reasoning: {levels}", { levels: w("low") + ", " + w("high") })), `levels: ${title}`);
        // no threshold of its own: follows Long conversations
        const other = await facts("Follows Long conversations");
        assert(other.includes(w("Compact at: follows Long conversations")), `follows: ${other}`);
        assert(other.includes(w("Concurrency: no limit set")), `no limit: ${other}`);
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
  test(engine + ": the hover says the price, the plugin's limit, and the typed window", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    loadI18N();
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
      await t.test(lang, async () => {
        const w = (key, vars) => tr(lang, key, vars);
        const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 }, reducedMotion: "reduce", locale: LANG_TAGS[lang] });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="providers"]').click();
        const row = (name) => page.locator("#providers .row.provider").filter({ has: page.locator(".name", { hasText: name }) }).first();
        const facts = async (provider, chip) => {
          await row(provider).locator(".name").click();
          const ed = page.locator("#modal .editor");
          await ed.waitFor();
          const c = ed.locator(".mchip.pick", { hasText: chip }).first();
          await c.waitFor();
          const title = await c.getAttribute("title");
          await ed.getByRole("button", { name: w("Cancel"), exact: true }).click();
          await ed.waitFor({ state: "detached" });
          return title;
        };
        // the concurrency in force is the plugin's when the user set none
        const plim = await facts("Plugin Limit", "glm-5");
        assert(plim.includes(w("Concurrency: {n} at once", { n: 2 })), `plugin limit: ${plim}`);
        assert(!plim.includes(w("Concurrency: no limit set")), `not "no limit": ${plim}`);
        assert(plim.includes(w("Keys and accounts may hold their own limits")), `per-account: ${plim}`);
        // the price as Usage counts it, and what its list says of it
        const pr = await facts("Priced", "glm-5");
        assert(pr.includes(w("Price: {in} in · {out} out per 1M tokens", { in: "$2", out: "$8" })), `price: ${pr}`);
        assert(!pr.includes(w("list price")), `the user's price is no list: ${pr}`);
        assert(pr.includes(w("Takes: {list}", { list: "text, image" })), `takes: ${pr}`);
        assert(pr.includes(w("Gives: {list}", { list: "text" })), `gives: ${pr}`);
        assert(pr.includes(w("Released {day}", { day: "2026-11-01" })), `released: ${pr}`);
        // a model with no price of its own is named at its list's
        const air = await facts("Priced", "air-5");
        assert(air.includes(w("Price: {in} in · {out} out per 1M tokens", { in: "$0.5", out: "$2" })), `list price: ${air}`);
        assert(air.includes(w("list price")), `named at its list's: ${air}`);
        // a window the user typed compacts whole (#1809)
        const tp = await facts("Typed Window", "glm-5");
        assert(tp.includes(w("Compact at {n} (the window you typed)", { n: "500K" })), `typed: ${tp}`);
        assert(!tp.includes(w("Compact at: follows Long conversations")), `not follows: ${tp}`);
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
  test(engine + ": Full window says the model compacts nowhere", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    loadI18N();
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh"]) {
      await t.test(lang, async () => {
        const w = (key, vars) => tr(lang, key, vars);
        const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 }, reducedMotion: "reduce", locale: LANG_TAGS[lang] });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang, { fullContext: true }));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="providers"]').click();
        const row = page.locator("#providers .row.provider").filter({ has: page.locator(".name", { hasText: "Own Threshold" }) }).first();
        await row.locator(".name").click();
        const ed = page.locator("#modal .editor");
        await ed.waitFor();
        const chip = ed.locator(".mchip.pick", { hasText: "glm-5" }).first();
        await chip.waitFor();
        const title = await chip.getAttribute("title");
        // Full window: the server hands Codex and Claude Code no compaction
        // window at all, whatever is set (codexcat.go:1091, agent/claude.go)
        assert(title.includes(w("Full window: no compaction")), `full window: ${title}`);
        assert(!title.includes(w("Compact at {n}", { n: "128K" })), `no threshold said: ${title}`);
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
}
