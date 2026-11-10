// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 拆分成多个 tab，方便分组，就避免滚动很久了. A provider's
// editor has four tabs above its fields: All (the default, every field as
// before), Connection (name, ID, base URL, endpoints), Accounts & routing
// (keys, accounts, routing) and Behaviour (models, headers, the advanced
// settings). A tab shows its group alone; All shows them all; the pick is
// kept for the next editor. Every group keeps the editor's own label column,
// as display:contents wrappers. English and Chinese, both engines; the API
// is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const relay = {
  id: "relay", name: "Relay", icon: "generic", chat: "https://relay.example.com/v1", responses: "", anthropic: "", catalog: "",
  models: [{ id: "m1", name: "m1", on: true }], agents: [], fallback: [], headers: {},
  key: { set: false }, keyList: [{ id: "k1", name: "Main", masked: "sk-…1", on: true }, { id: "k2", name: "Backup", masked: "sk-…2", on: true }],
  balanceToken: { takes: false, set: false }, proxy: "",
};

function serve(lang) {
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
  en: { all: "All", conn: "Connection", acct: "Accounts & routing", behave: "Behaviour", accounts: "Accounts", models: "Models", advanced: "Advanced" },
  zh: { all: "全部", conn: "连接", acct: "账号与路由", behave: "行为", accounts: "账号", models: "模型", advanced: "高级" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: a provider editor's fields are tabbed in three groups`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const ctx = await browser.newContext({ viewport: { width: 980, height: 820 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
      const page = await ctx.newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", serve(lang));
      await page.goto("http://magpie.test/?view=providers");
      const open = async () => {
        await page.locator(".row.provider", { hasText: "Relay" }).click();
        await page.locator(".editor .edtabs").waitFor();
      };
      const tab = (name) => page.locator(".edtabs button", { hasText: name }).first();
      const label = (name) => page.locator(".editor label", { hasText: name }).first();
      const baseUrl = () => page.locator(".editor .base-url").first();
      const shown = async (name) => label(name).isVisible();
      const modelsShown = () => page.locator(".editor .mchips").first().isVisible();
      await open();
      // All is the default, every field as before
      const names = await page.locator(".edtabs button").allTextContents();
      assert.deepEqual(names.map((x) => x.trim()), [w.all, w.conn, w.acct, w.behave]);
      assert.equal(await tab(w.all).evaluate((b) => b.classList.contains("on")), true);
      assert(await baseUrl().isVisible(), "Connection's base URL");
      assert(await shown(w.accounts), "Accounts");
      assert(await modelsShown(), "Models");
      assert(await page.locator(".editor details.advanced summary", { hasText: w.advanced }).isVisible());

      // Connection alone
      await tab(w.conn).click();
      assert.equal(await tab(w.conn).getAttribute("aria-pressed"), "true");
      assert(await baseUrl().isVisible());
      assert.equal(await shown(w.accounts), false, "Connection hides Accounts");
      assert.equal(await modelsShown(), false, "Connection hides Models");
      assert.equal(await page.locator(".editor details.advanced").isVisible(), false, "Connection hides Advanced");

      // Accounts & routing alone
      await tab(w.acct).click();
      assert(await shown(w.accounts), "Accounts");
      assert.equal(await baseUrl().isVisible(), false, "Accounts hides Connection");
      assert.equal(await modelsShown(), false);

      // Behaviour alone
      await tab(w.behave).click();
      assert(await modelsShown(), "Models");
      assert(await page.locator(".editor details.advanced").isVisible(), "Advanced");
      assert.equal(await baseUrl().isVisible(), false);
      assert.equal(await shown(w.accounts), false);

      // All again
      await tab(w.all).click();
      assert(await baseUrl().isVisible());
      assert(await shown(w.accounts));
      assert(await modelsShown());

      // the pick is kept for the next editor
      await tab(w.behave).click();
      await page.locator(".editor .bar button", { hasText: lang === "zh" ? "取消" : "Cancel" }).click();
      await page.locator(".editor").waitFor({ state: "detached" });
      await open();
      assert.equal(await tab(w.behave).getAttribute("aria-pressed"), "true", "Behaviour is remembered");
      assert(await modelsShown());
      assert.equal(await baseUrl().isVisible(), false);

      // the labels still share the editor's one column: a row's label and its
      // control are in the same two columns whichever group is shown
      const cols = await page.locator(".editor .ebody").evaluate((b) => getComputedStyle(b).gridTemplateColumns.split(" ").length);
      assert.equal(cols, 2, "two columns");
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  }
}
