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

function serve(lang) {
  const providers = { providers: [own, follows], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light", compactAt: 0 } };
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
}
