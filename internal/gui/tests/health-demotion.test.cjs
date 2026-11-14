// Run with Node's test runner and Playwright on the module path; see README.md.
// Settings › Models › Unstable models after the steady ones (gateway
// health.go): on by default — an intermittently failing model is tried
// after the steady ones of its plan — Off posts settings/health-demotion
// on its own and lights Off, On posts it back, and neither click scrolls
// the Settings page. English and Chinese, Chromium and WebKit; the API is
// faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { watchSettingsReads, settingsDrawn, wheelTo } = require("./reader.cjs");

const assets = path.resolve(__dirname, "../assets");

function settingsPayload(over) {
  return {
    theme: "light", lang: "en", tray: "panel", quotaLeft: false, currency: "usd",
    dock: false, dockWindow: false, proxy: "", redact: false, redactPersonal: false, redactWords: [],
    codexWarmup: "", claudeWarmup: "", codexWarmAt: "", claudeWarmAt: "", workbuddyCheckin: false, noStats: false,
    trayUsage: "", trayUsageEvery: 3, vision: "", imageGen: "",
    version: "0.1.1220", dir: "~/.config/magpie", gateway: "http://127.0.0.1:3425",
    proxyNow: "none", proxySource: "none", login: false,
    visionModels: [], imageGenModels: [], workbuddyCheckins: [], lan: false, lanURLs: [],
    fx: { rate: 7.2, at: new Date().toISOString(), stale: false },
    ...over,
  };
}

function server(lang, posted) {
  let cur = settingsPayload({ lang });
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === "/api/window/fit") return route.fulfill({ status: 204 });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" }, fx: cur.fx });
    if (url.pathname === "/api/settings") {
      if (req.method() === "POST") {
        const body = req.postDataJSON();
        posted.push(body);
        if ("noHealthDemotion" in body) cur = { ...cur, noHealthDemotion: body.noHealthDemotion };
      }
      return json(cur);
    }
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const want = {
  en: { name: "Unstable models after the steady ones", sub: "tried after the steady ones of its plan", off: "Off", on: "On" },
  zh: { name: "不稳定的模型排在稳定模型之后", sub: "排到同一计划里稳定模型之后", off: "关闭", on: "开启" },
};

const view = (page) => page.locator("#view-settings").evaluate((v) => v.scrollTop);

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of Object.keys(want)) {
    const w = want[lang];
    test(`${engine} ${lang}: unstable models are tried after the steady ones, or not`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 900, height: 700 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posted = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang, posted));
      await watchSettingsReads(page);
      await page.goto("http://magpie.test/?view=settings&tab=models");
      await settingsDrawn(page);
      const segs = page.locator("#healthDemotionRow .opt");
      await segs.first().waitFor();
      const row = page.locator("#healthDemotionRow");
      assert(await row.isVisible(), "the row is drawn");
      assert.equal((await row.locator(".name").textContent()).trim(), w.name);
      assert((await row.locator(".sub").textContent()).includes(w.sub), "the sub says what it does");
      assert.deepEqual((await segs.allTextContents()).map((s) => s.trim()), [w.off, w.on]);
      assert.equal(await segs.nth(1).evaluate((b) => b.classList.contains("on")), true, "on by default");
      assert.equal(await segs.nth(0).evaluate((b) => b.classList.contains("on")), false, "off is not lit");
      assert.equal(await page.locator("#healthDemotionRow select").count(), 0, "no native select");
      const border = await row.evaluate((r) => getComputedStyle(r).borderLeftWidth);
      assert(border === "0px" || border === "", "no left border accent: " + border);
      assert(await page.locator("#view-settings").evaluate((v) => v.scrollWidth <= v.clientWidth + 1), "the settings scroll sideways");

      // Off moves nothing on the page and is posted with the save: the
      // wheel brings the row in first, so the click's own scroll is the
      // one measured
      await wheelTo(page, segs.nth(0), 200);
      const before = await view(page);
      assert(before > 0, "the settings list must scroll to the row");
      await segs.nth(0).click();
      await page.locator("#healthDemotionRow .opt.on", { hasText: w.off }).waitFor();
      await page.waitForTimeout(400);
      assert.equal(await view(page), before, "the click scrolled the page");
      assert.equal(posted.length, 1, "one save");
      assert.equal(posted[0].noHealthDemotion, true, "the save carries it");

      // back on
      await segs.nth(1).click();
      await page.locator("#healthDemotionRow .opt.on", { hasText: w.on }).waitFor();
      assert.equal(posted.at(-1).noHealthDemotion, false, "off again");
      assert.equal(posted.length, 2);
      assert.equal(await view(page), before, "the click scrolled the page");
      assert.deepEqual(errors, []);
    });
  }
}
