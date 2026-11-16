// Run with Node's test runner and Playwright on the module path; see README.md.
// A rest the gateway gains or lifts reaches the Providers page only with a
// whole read, which the 5s hold delays. The lanes' two-second tick carries
// the rests in force (/api/lanes rests, each rest key to until in
// milliseconds), and a key gained or gone from it asks for the rows at once:
// the row's rest badge appears while the page stands open, without a
// reload, and goes when the rest does. A rest's until alone moving is not a
// change, and no row is rewritten in place (the editor and the scroll stay).
// In every language, on Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");

function serve(lang, lanes, providersReads) {
  // the relay as /api/providers answers it now: the rest rides its first
  // key only while the gateway holds it, so every read is built fresh
  const relay = () => ({
    id: "relay", name: "Relay", icon: "generic", chat: "https://relay.example.com/v1", responses: "", anthropic: "", catalog: "",
    models: [{ id: "model-a", name: "Model A", on: true }], agents: [], fallback: [], headers: {},
    key: { set: true, masked: "sk-…one" },
    keyList: lanes.restOn
      ? [{ id: "k1", name: "Main", masked: "sk-…one", on: true, rest: { why: "rate", status: 429, until: lanes.until, key: "relay#k1" } }]
      : [{ id: "k1", name: "Main", masked: "sk-…one", on: true }],
    balanceToken: { takes: false, set: false }, proxy: "", maxConcurrency: 2,
  });
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") { providersReads.whole++; return json({ providers: [relay()], presets: [], excluded: [], gateway: { running: true, window: true, lanes: {} } }); }
    // the rests in force now, as the gateway's memory holds them; the lanes
    // ride beside them, as the real answer does
    if (url.pathname === "/api/lanes") {
      lanes.reads++;
      return json({ lanes: {}, rests: lanes.restOn ? { "relay#k1": Date.parse(lanes.until) } : null });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

// the app's own words, as t() gives them
const say = (page, lang, key, vars = {}) => page.evaluate(([lang, key, vars]) => ((lang !== "en" && I18N[lang]?.[key]) || key).replace(/\{(\w+)\}/g, (_, k) => vars[k]), [lang, key, vars]);

// waits up to ms for fn to hold, checking every step: the seconds it took,
// or -1 when it never did
const waited = async (ms, fn, step = 100) => {
  const at = Date.now();
  for (let i = 0; i < ms; i += step) {
    if (await fn()) return Date.now() - at;
    await new Promise((r) => setTimeout(r, step));
  }
  return -1;
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    test(`${engine} ${lang}: a rest gained or lifted reaches the rows' badge without a reload`, async (t) => {
      const lanes = { restOn: false, until: new Date(Date.now() + 3600e3).toISOString(), reads: 0 };
      const providersReads = { whole: 0 };
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 1100, height: 800 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, lanes, providersReads));
      await page.goto("http://magpie.test/?view=providers");
      const row = page.locator(".row.provider", { hasText: "Relay" }).first();
      await row.click();
      // the editor open puts the lanes' tick to work: a pill in sight
      await page.locator(".editor .accts .acc .alane").first().waitFor({ state: "attached" });
      await page.evaluate(() => { window.restLivePage = true; });
      const badge = page.locator(".row.provider .badge.rest-now");
      assert.equal(await badge.count(), 0, "nothing rests yet");
      // the tick reads the lanes first: the rests are learned before a
      // change in them can be seen
      await waited(8000, () => lanes.reads > 0);
      assert(lanes.reads > 0, "the lanes were read");
      const reads = lanes.reads, wholes = providersReads.whole;

      // the gateway rests Main: the tick sees the key gained and asks for
      // the rows, whose fresh read says the rest — the badge, on the page
      // it stood on all along
      const at = Date.now();
      lanes.restOn = true;
      const ms = await waited(8000, () => badge.count().then((n) => n > 0));
      assert(ms >= 0, "the badge never came");
      assert(Date.now() - at < 8000, `the badge took ${Date.now() - at}ms`);
      assert.equal(await badge.textContent(), await say(page, lang, "{n} resting", { n: 1 }));
      assert(lanes.reads > reads, "the tick read the lanes again");
      assert(providersReads.whole > wholes, "the rows were asked whole, not rewritten in place");
      assert.equal(await page.evaluate(() => window.restLivePage), true, "the page stood open");

      // the rest ends: the key gone from the summary asks for the rows
      // again, and the badge goes
      const wholes2 = providersReads.whole;
      lanes.restOn = false;
      assert((await waited(8000, () => badge.count().then((n) => n === 0))) >= 0, "the badge never went");
      assert(providersReads.whole > wholes2, "the rows were asked whole again");
      assert.equal(await page.evaluate(() => window.restLivePage), true, "still no reload");
      assert.deepEqual(errors, []);
    });
  }
}
