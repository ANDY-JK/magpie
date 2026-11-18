// Run with Node's test runner and Playwright on the module path; see README.md.
// A vendor's model list can change under the user: a scheduled refresh (or
// Fetch models) drops a model, and magpie used to say so only in its log.
// Now the provider keeps the last change (added/removed ids and when), and
// the Providers row and its editor say the models gone from the list in
// amber, up to five names and how long ago (all of them in the title). A
// routing group's card says a member nothing serves now, rather than only
// "no member ready" when every one is gone. English and the four
// translations, Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const at = "2026-11-17T09:30:00Z";
const removed = ["old-1", "old-2", "old-3", "old-4", "old-5", "old-6", "old-7"];

const relay = {
  id: "relay", name: "Relay", icon: "generic", chat: "https://relay.example.com/v1", responses: "", anthropic: "", catalog: "",
  host: "relay.example.com",
  models: [{ id: "model-a", name: "Model A", on: true }], agents: [], fallback: [], headers: {},
  key: { set: true, masked: "sk-…one" }, keyList: [], balanceToken: { takes: false, set: false }, proxy: "", maxConcurrency: null,
  listChange: { added: ["new-1"], removed, at },
  fetched: at,
};
const steady = {
  ...relay, id: "steady", name: "Steady", host: "steady.example.com", listChange: undefined,
};

function providersServe(lang) {
  const providers = { providers: [relay, steady], presets: [], excluded: [], gateway: { running: true, window: true } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

// a group naming one model that is served and one nothing serves now
const group = {
  id: "mine", name: "Mine", ref: "group/mine", ready: true,
  members: ["a/m", "a/gone"], off: [], holds: [], offers: [], shared: [], patterns: [],
  memberInfo: [{ id: "a/m", ready: true }, { id: "a/gone", ready: false }],
};

function routingServe(lang) {
  const models = [{ id: "a/m", name: "m", providerName: "A", icon: "generic" }];
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/groups") return json({ models, pools: [], deciders: [], found: true, groups: [group] });
    if (url.pathname === "/api/gateway/trace") {
      if (url.searchParams.get("wait")) return new Promise(() => {}); // nothing more comes
      return json({ mine: true, now: new Date().toISOString(), seq: 1, totals: { requests: 0, rerouted: 0, errors: 0 }, routes: [] });
    }
    if (url.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true, url: "http://127.0.0.1:3999" } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

// the app's own words, as t() gives them
const say = (page, lang, key, vars = {}) => page.evaluate(([lang, key, vars]) => ((lang !== "en" && I18N[lang]?.[key]) || key).replace(/\{(\w+)\}/g, (_, k) => vars[k]), [lang, key, vars]);

async function open(engine, lang, t, name) {
  const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
  const page = await (await browser.newContext({ viewport: { width: 1100, height: 800 }, reducedMotion: "reduce", colorScheme: "light" })).newPage();
  t.after(async () => {
    if (process.env.ARTIFACT_DIR) {
      await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
      await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, name) });
    }
    await browser.close();
  });
  page.setDefaultTimeout(5000);
  page.setDefaultNavigationTimeout(60000);
  return page;
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    test(`${engine} ${lang}: a provider's row and editor say the models its last list change dropped`, async (t) => {
      const page = await open(engine, lang, t, `${engine}-${lang}-provider-list-change.png`);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", providersServe(lang));
      await page.goto("http://magpie.test/?view=providers");

      const row = page.locator(".row.provider", { hasText: "Relay" }).first();
      await row.waitFor({ state: "visible" });
      const line = row.locator(".sub.list-change");
      assert.equal(await line.count(), 1, "the row says the drop");
      const when = await page.evaluate((at) => ago(at), at);
      const shown = removed.slice(0, 5).join(", ") + " …";
      assert.equal(await line.textContent(), await say(page, lang,
        "Last change to the vendor's list: dropped {models} · {when}", { models: shown, when }));
      const title = await line.getAttribute("title");
      assert(title.includes(removed.join(", ")), `the title names every dropped model: ${title}`);
      assert(!title.includes("new-1"), "models added are not in the warning");

      // amber, the warning colour
      const color = await line.evaluate((n) => getComputedStyle(n).color);
      const amber = await page.evaluate(() => {
        const d = document.createElement("div");
        d.style.color = getComputedStyle(document.documentElement).getPropertyValue("--amber").trim();
        document.body.append(d);
        const c = getComputedStyle(d).color;
        d.remove();
        return c;
      });
      assert.equal(color, amber, "the line is amber");

      // the provider whose list did not change has none
      assert.equal(await page.locator(".row.provider", { hasText: "Steady" }).locator(".sub.list-change").count(), 0, "no change, no line");

      // the editor says it too when opened
      await row.click();
      const inEditor = page.locator(".editor .model-hint.list-change");
      await inEditor.waitFor({ state: "visible" });
      assert.equal(await inEditor.textContent(), await line.textContent(), "the editor repeats the row's words");
      assert.deepEqual(errors, []);
    });

    test(`${engine} ${lang}: a routing group's card names a member nothing serves now`, async (t) => {
      const page = await open(engine, lang, t, `${engine}-${lang}-routing-stale-member.png`);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", routingServe(lang));
      await page.goto("http://magpie.test/?view=routing");

      const card = page.locator(".rt-group", { hasText: "Mine" }).first();
      await card.waitFor({ state: "visible" });
      const stale = card.locator(".mem.rt-stale");
      assert.equal(await stale.count(), 1, "the card says the member is gone");
      assert.equal(await stale.textContent(), await say(page, lang,
        "Member {member} is no longer in the list", { member: "a/gone" }));
      const color = await stale.evaluate((n) => getComputedStyle(n).color);
      const amber = await page.evaluate(() => {
        const d = document.createElement("div");
        d.style.color = getComputedStyle(document.documentElement).getPropertyValue("--amber").trim();
        document.body.append(d);
        const c = getComputedStyle(d).color;
        d.remove();
        return c;
      });
      assert.equal(color, amber, "the member line is amber");
      assert.deepEqual(errors, []);
    });
  }
}
