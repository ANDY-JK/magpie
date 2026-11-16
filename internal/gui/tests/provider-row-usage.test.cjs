// Run with Node's test runner and Playwright on the module path; see README.md.
// A provider's row says what it was asked today and how its models are
// answering: the today pill ("Today 7 calls · $12.50") comes from the
// Usage ranking's own period (today, the reader's zone), read every 30s
// at most; the health badge ("2 unstable models", amber, red when every
// model with a record is one of them) comes with the providers read, its
// hover naming the least steady model and how much of what it was asked
// it answered. A provider with no calls today, one whose entry holds no
// calls, and one whose models have no record draw nothing. In every
// language, on Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");

const rank = () => ({
  period: "today",
  since: new Date().toISOString(),
  tier: { needs: 5, days: 7, calls: 100, errors: 0.2, still: 7 },
  providers: [
    totals("relay", 7, 12.5),
    totals("down", 3, 0.5),
    totals("zero", 0, 0),
  ],
  thin: [],
});
// one provider's place in the ranking, as /api/usage/rank answers it: the
// totals a day of calls sums to
function totals(id, calls, cost) {
  return { id, name: id, icon: "generic", rank: 1, score: 90, grade: "A",
    calls, errors: 0, input: calls * 100, output: calls * 10, cache_read: 0, cache_write: 0,
    reasoning: 0, cost, unpriced: 0 };
}

function provider(id, health) {
  return {
    id, name: id.charAt(0).toUpperCase() + id.slice(1), icon: "generic",
    chat: `https://${id}.example.com/v1`, host: `${id}.example.com`, responses: "", anthropic: "", catalog: "",
    models: [{ id: "model-a", name: "Model A", on: true }], agents: [], fallback: [], headers: {},
    key: { set: true, masked: "sk-…one" },
    keyList: [{ id: "k1", name: "Main", masked: "sk-…one", on: true }],
    balanceToken: { takes: false, set: false }, proxy: "", maxConcurrency: 2,
    health,
  };
}

function serve(lang) {
  const providers = [
    // asked today, and one model under the line: pill, amber badge
    provider("relay", { worst: 0.2, worstModel: "model-a", below: 1, recorded: 4 }),
    // asked today, and every model with a record under the line: red
    provider("down", { worst: 0.25, worstModel: "model-b", below: 2, recorded: 2 }),
    // steady, with records but none below: no badge, and nothing today
    provider("fine", { worst: 0.9, worstModel: "model-c", below: 0, recorded: 6 }),
    // never asked, never used: neither
    provider("quiet", undefined),
    // in the ranking with no calls: no pill
    provider("zero", undefined),
  ];
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json({ providers, presets: [], excluded: [], gateway: { running: true, window: true, lanes: {} } });
    if (url.pathname === "/api/usage/rank") return json(rank());
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
    test(`${engine} ${lang}: a row's today pill and health badge say what it was asked and how it answers`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 1100, height: 800 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang));
      await page.goto("http://magpie.test/?view=providers");

      const row = (id) => page.locator(".row.provider", { hasText: id.charAt(0).toUpperCase() + id.slice(1) }).first();
      await row("relay").waitFor({ state: "visible" });
      const pill = page.locator(".badge.today-calls");
      assert((await waited(10000, () => pill.count())) >= 0, "the pills never came");
      assert.equal(await pill.count(), 2, "relay's and down's pills alone");
      const relayPill = await row("relay").locator(".badge.today-calls").textContent();
      assert.equal(relayPill, (await say(page, lang, "Today {n} calls", { n: 7 })) + " · $12.50");
      const downPill = await row("down").locator(".badge.today-calls").textContent();
      assert.equal(downPill, (await say(page, lang, "Today {n} calls", { n: 3 })) + " · $0.500");

      // the health badges: amber while a steady model stands beside the
      // unstable one, red when none does, hover saying which is worst
      const amber = row("relay").locator(".badge.hc-unstable");
      assert.equal(await amber.count(), 1, "relay's badge");
      assert.equal(await amber.getAttribute("class"), "badge hc-unstable", "amber while a steady model stands beside");
      assert.equal(await amber.textContent(), await say(page, lang, "{n} unstable models", { n: 1 }));
      assert.equal(await amber.getAttribute("title"), await say(page, lang,
        "{model} is the least steady of this provider's models just now: {pct}% of its calls succeed", { model: "model-a", pct: 20 }));
      const red = row("down").locator(".badge.hc-unstable.bad");
      assert.equal(await red.count(), 1, "down's badge is red");
      assert.equal(await red.textContent(), await say(page, lang, "{n} unstable models", { n: 2 }));

      // records but none below the line: no badge; nothing recorded: no
      // badge; in the ranking with no calls: no pill
      assert.equal(await row("fine").locator(".badge.hc-unstable").count(), 0, "a steady provider draws no badge");
      assert.equal(await row("quiet").locator(".badge.hc-unstable").count(), 0, "a never-asked provider draws no badge");
      assert.equal(await row("quiet").locator(".badge.today-calls").count(), 0, "a never-used provider draws no pill");
      assert.equal(await row("zero").locator(".badge.today-calls").count(), 0, "no pill for a day of no calls");
      assert.deepEqual(errors, []);
    });
  }
}
