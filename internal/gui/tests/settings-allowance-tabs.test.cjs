// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 拆分成多个 tab，避免滚动很久. Settings → Allowances has
// its own tabs above its rows — All (the default), Menu bar, Display and
// alerts, Warm-ups — a pick showing that group alone and remembered; All is
// the part as it was, so nothing changes for a reader who doesn't pick.
// English and Chinese, Chromium and WebKit; the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
function settingsPayload(over) {
  return {
    theme: "light", lang: "en", tray: "panel", quotaLeft: false, currency: "usd", textSize: 100,
    dock: false, dockWindow: false, proxy: "", redact: false, redactPersonal: false, redactWords: [],
    codexWarmup: "", claudeWarmup: "", codexWarmAt: "", claudeWarmAt: "", workbuddyCheckin: false, noStats: false,
    trayUsage: "", trayUsageEvery: 3, vision: "", imageGen: "",
    version: "0.1.400", dir: "~/.config/magpie", gateway: "http://127.0.0.1:3425",
    proxyNow: "none", proxySource: "none", login: false,
    visionModels: [], imageGenModels: [], workbuddyCheckins: [], lanURLs: [],
    fx: { rate: 7.2, at: new Date().toISOString(), stale: false },
    ...over,
  };
}
function server(lang, posts) {
  let cur = settingsPayload({ lang });
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" }, fx: cur.fx });
    if (url.pathname === "/api/settings") {
      if (req.method() === "POST") { const body = req.postDataJSON(); posts.push(body); cur = { ...cur, ...body }; }
      return json(cur);
    }
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname === "/api/window/fit") return route.fulfill({ status: 204 });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}
const L = { en: { all: "All", menu: "Menu bar", display: "Display and alerts", warm: "Warm-ups" }, zh: { all: "全部", menu: "菜单栏", display: "显示与提醒", warm: "预热" } };

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: Allowances' tabs show a group alone`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1000, height: 720 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(15000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang, posts));
      await page.goto("http://magpie.test/?view=settings&tab=usage");
      const tabs = page.locator("#usageTabs");
      await page.waitForTimeout(800);
      console.log("DBG", JSON.stringify(await page.evaluate(() => ({
        has: !!document.querySelector("#usageTabs"),
        page: !!document.querySelector("#setPage-usage"),
        scripts: [...document.scripts].map((s) => s.src.split("/").pop()),
      }))), "ERR", JSON.stringify(errors));
      await tabs.waitFor();
      const names = await tabs.locator("button:not([hidden])").allTextContents();
      assert.deepEqual(names.map((x) => x.trim()), [w.all, w.menu, w.display, w.warm], "All and the three groups");
      const shown = (sel) => page.locator(sel).evaluate((e) => !!e.getClientRects().length);
      // All: the menu bar's rows, the display's and the warm-ups'
      assert.equal(await shown("#trayUsageRow"), true, "the menu bar's rows");
      assert.equal(await shown("#currencySegs"), true, "the display's");
      assert.equal(await shown("#warmTabs"), true, "the warm-ups'");
      // Display and alerts: the other two go
      await tabs.locator('button[data-aset="display"]').click();
      await page.waitForTimeout(400);
      assert.equal(await shown("#trayUsageRow"), false, "the menu bar's rows go");
      assert.equal(await shown("#currencySegs"), true, "the display's stay");
      assert.equal(await shown("#warmTabs"), false, "the warm-ups go");
      // Warm-ups: remembered across a reload
      await tabs.locator('button[data-aset="warm"]').click();
      await page.waitForTimeout(300);
      await page.reload();
      await page.locator("#usageTabs").waitFor();
      await page.waitForTimeout(500);
      assert.equal(await shown("#warmTabs"), true, "Warm-ups is still the pick");
      assert.equal(await shown("#currencySegs"), false, "the display's are out of the way");
      // All brings everything back
      await page.locator('#usageTabs button[data-aset="all"]').click();
      await page.waitForTimeout(400);
      assert.equal(await shown("#trayUsageRow"), true);
      assert.equal(await shown("#currencySegs"), true);
      assert.equal(await shown("#warmTabs"), true);
      assert.deepEqual(errors, []);
      await page.context().close();
    });
  }
}
