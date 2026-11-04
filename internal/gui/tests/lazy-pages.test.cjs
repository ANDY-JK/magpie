// Run with Node's test runner and Playwright on the module path; see README.md.
// The heavy pages are declared before their scripts (pages.js, kernel.js's
// magpie.lazy): the page and its view are in the window from the start, and
// its script, markup and hooks come the first time the reader opens it. An
// English page that opens neither Sessions nor Library fetches neither one's
// code. Routing is not one of them: its script hands the tray panel and
// Usage's Requests what they draw with (pages.js).
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const LAZY = ["plugins/sessions/client.js", "plugins/library/client.js"];

function serve() {
  return async (r) => {
    const url = new URL(r.request().url());
    const json = (data) => r.fulfill({ json: data });
    const settings = { lang: "en", theme: "light" };
    if (url.pathname === "/boot.js") return r.fulfill({ contentType: "text/javascript", body: 'window.bootPrefs = {lang:"en",theme:"light",web:false};' });
    if (url.pathname === "/wails/runtime.js") return r.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") return json(settings);
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true, url: "http://127.0.0.1:3999" } });
    if (url.pathname === "/api/groups") return json({ models: [], groups: [], pools: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/library") return json({ agents: [], servers: [], skills: [], hooks: [], subagents: [] });
    if (url.pathname === "/api/gateway/trace") return json({ mine: true, now: new Date().toISOString(), seq: 0, totals: { requests: 0, rerouted: 0, errors: 0 }, routes: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await r.fulfill({ body: await fs.readFile(file), contentType }); } catch { await r.fulfill({ status: 404 }); }
  };
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: a heavy page's code comes when the page is opened, not before`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    const page = await (await browser.newContext({ viewport: { width: 1100, height: 800 } })).newPage();
    page.setDefaultTimeout(15000);
    page.setDefaultNavigationTimeout(60000);
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const asked = [];
    page.on("request", (r) => {
      const u = new URL(r.url()).pathname.slice(1);
      if (LAZY.includes(u)) asked.push(u);
    });
    await page.route("**/*", serve());
    await page.goto("http://magpie.test/");
    await page.locator("#agents .row, #agents .empty-state").first().waitFor();

    // the pages are the window's from the start: the sidebar lists them,
    // show() hides and shows their views, and their titles are here
    const before = await page.evaluate(() => ({
      pages: magpie.pages().sort(),
      titles: ["sessions", "library", "routing"].map((k) => magpie.page(k)?.title),
      views: ["sessions", "library", "routing"].map((k) => !!document.getElementById("view-" + k)),
      empties: ["sessions", "library"].map((k) => document.getElementById("view-" + k).children.length),
      nav: [...document.querySelectorAll("#nav [data-view]")].map((b) => b.dataset.view),
      dirty: magpie.call("routing", "dirty"),
    }));
    assert.deepEqual(before.pages.filter((k) => ["sessions", "library", "routing"].includes(k)), ["library", "routing", "sessions"]);
    assert.deepEqual(before.titles, ["Sessions", "Library", "Routing"]);
    assert.deepEqual(before.views, [true, true, true]);
    assert.deepEqual(before.empties, [0, 0], "a page not opened yet has drawn nothing");
    assert.deepEqual(before.nav.filter((v) => ["sessions", "library", "routing"].includes(v)), ["sessions", "library", "routing"]);
    assert.deepEqual(asked, [], "no heavy page's code is fetched before one is opened");
    // what a page that hasn't come says of unsaved work is not "true": the
    // window's guards ask with === true (app.js)
    assert.notEqual(before.dirty, true);
    // Routing's script is the window's: it isn't self-contained (pages.js)
    assert.equal(await page.evaluate(() => document.querySelectorAll('script[src*="routing/client.js"]').length), 1);
    assert.equal(await page.evaluate(() => typeof window.kindName), "function");

    // the reader opens Library: its script comes, and the page draws
    await page.locator('#nav > button[data-view="library"]').click();
    await page.locator("#view-library .lib-head").waitFor();
    assert.deepEqual(asked, ["plugins/library/client.js"]);
    assert.deepEqual(errors, []);
    // the rest stay unasked until their pages are
    assert.deepEqual(asked.filter((u) => u !== "plugins/library/client.js"), []);
    await page.close();
  });

  test(`${engine}: a page opened twice brings its code in once`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    const page = await (await browser.newContext({ viewport: { width: 1100, height: 800 } })).newPage();
    page.setDefaultTimeout(15000);
    page.setDefaultNavigationTimeout(60000);
    const asked = [];
    page.on("request", (r) => {
      const u = new URL(r.url()).pathname.slice(1);
      if (LAZY.includes(u)) asked.push(u);
    });
    await page.route("**/*", serve());
    await page.goto("http://magpie.test/");
    await page.locator("#agents .row, #agents .empty-state").first().waitFor();
    // two asks in the same turn are one fetch: the page's script is brought
    // in once, and both get the page ready
    assert.deepEqual(await page.evaluate(() => Promise.all([magpie.ready("library"), magpie.ready("library")])), [true, true]);
    assert.equal(await page.evaluate(() => document.querySelectorAll('script[src*="library/client.js"]').length), 1, "one script element, however many asked");
    assert.deepEqual(asked, ["plugins/library/client.js"]);
    for (let i = 0; i < 3; i++) {
      await page.locator('#nav > button[data-view="library"]').click();
      await page.waitForFunction(() => document.getElementById("view-library").children.length > 0);
      await page.locator('#nav > button[data-view="agents"]').click();
      await page.locator("#agents .row, #agents .empty-state").first().waitFor();
    }
    assert.deepEqual(asked, ["plugins/library/client.js"]);
    // ready() for a page whose script has run answers at once
    assert.equal(await page.evaluate(() => magpie.ready("library")), true);
  });
}
