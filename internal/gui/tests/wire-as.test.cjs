// Run with Node's test runner and Playwright on the module path; see README.md.
// The editor's Names & levels gains a Wire as box (the owner, 诉求8): the
// per-model setting that only `magpie model wire` could set until now — the
// name the request to the vendor carries, the id agents see unchanged. The
// box shows what is saved, stages with the editor's Save (nothing goes out
// before it), and is not drawn on a provider magpie reaches through its
// agent's own backend, where no model field goes out to rename
// (provider.asksOwnBackend). English and Chinese; the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const model = (id, more = {}) => ({ id, name: id, on: true, context: 100000, images: false, efforts: [], ...more });
const base = { icon: "generic", chat: "http://127.0.0.1:8317/v1", responses: "", anthropic: "", catalog: "", models: [], agents: [], fallback: [], headers: {}, balanceToken: { takes: false, set: false }, proxy: "", searches: false, key: { set: true, masked: "sk-…aaa" }, keyList: [], ready: true };
const relay = { ...base, id: "relay", name: "Relay", models: [model("sol", { wires: true, wire: "vendor-c/sol" })] };
const sub = { ...base, id: "sub", name: "Signed in", wires: false, account: { agent: "claude", agentName: "Claude", logins: [] }, models: [model("sol")] };

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

function serve(lang, posts) {
  const providers = { providers: [relay, sub], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light", compactAt: 0 } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (route.request().method() === "POST" && url.pathname === "/api/provider/save") {
      posts.push(route.request().postDataJSON());
      return json(providers);
    }
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": Names & levels has a Wire as box that stages with the Save", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    loadI18N();
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh"]) {
      await t.test(lang, async () => {
        const w = (key, vars) => tr(lang, key, vars);
        const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        const errors = [], posts = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang, posts));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="providers"]').click();
        const row = (name) => page.locator("#providers .row.provider").filter({ has: page.locator(".name", { hasText: name }) }).first();
        const names = async (provider) => {
          await row(provider).locator(".name").click();
          const ed = page.locator("#modal .editor");
          await ed.waitFor();
          await ed.getByRole("button", { name: w("Names & levels"), exact: true }).click();
          return ed;
        };
        // the saved wire is what the box shows
        const ed = await names("Relay");
        const box = ed.locator("label.msame", { hasText: w("Wire as") }).first();
        await box.waitFor();
        assert.equal(await box.locator("input").inputValue(), "vendor-c/sol", "the saved wire is shown");
        // staged, nothing sent, and the Save carries it
        await box.locator("input").fill("vendor-c/newname");
        assert.deepEqual(posts, [], "nothing goes out before the Save");
        await ed.getByRole("button", { name: w("Save"), exact: true }).click();
        for (let i = 0; i < 60 && !posts.length; i++) await page.waitForTimeout(50);
        assert.equal(posts.length, 1, "one save");
        assert.equal(posts[0].modelPrefs?.sol?.wire, "vendor-c/newname", "the wire is in the save");
        // a save closes the editor
        await ed.waitFor({ state: "detached" });
        // a provider magpie reaches through its agent's own backend has no
        // box: no model field goes out there to rename
        const ed2 = await names("Signed in");
        assert.equal(await ed2.locator("label.msame", { hasText: w("Wire as") }).count(), 0, "no Wire as on an own-backend provider");
        await ed2.getByRole("button", { name: w("Cancel"), exact: true }).click();
        await ed2.waitFor({ state: "detached" });
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
}
