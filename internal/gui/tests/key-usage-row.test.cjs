// Run with Node's test runner and Playwright on the module path; see README.md.
// The editor's key rows say what the key was asked today and what allowance
// magpie has read for it: a day line ("Today 7 calls · $12.50 · 700 in ·
// 70 out", the Usage page's Today by key, from the summary's own key groups,
// its own read every 30s at most), and the key's card drawn only from the
// cards already read — the routing view's read among them — opening the
// editor never asking a vendor. A key with no calls today, and one no card
// names, draw nothing; a key whose records keep an older fingerprint draws
// its day by its name. A card's refresh reads that card alone (provider and
// user), and no read of the allowances goes out with the editor. In every
// language, Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");

const duo = () => ({
  id: "duo", name: "Duo", icon: "generic", chat: "https://duo.example.com/v1", responses: "", anthropic: "", catalog: "",
  models: [{ id: "gpt-5.5", name: "", on: true }], agents: [], fallback: [], headers: {}, routing: "",
  key: { set: true, masked: "sk-…one" }, balanceToken: { takes: false, set: false }, proxy: "",
  keyList: [
    { id: "aaaa111111", name: "Main", masked: "sk-…one", on: true, active: true },
    { id: "bbbb222222", name: "Spare", masked: "sk-…two", on: true },
    { id: "cccc333333", name: "", masked: "sk-…three", on: true },
  ],
});
// the Usage summary's key groups, as /api/usage answers them: keyed
// provider#fingerprint, the key's name and the day's totals beside. Spare's
// group keeps the fingerprint of a value since replaced, so its row finds
// it by name; the unnamed key has no group and no card at all.
const keyGroups = () => [
  { id: "duo#aaaa111111", provider: "duo", providerKeyId: "aaaa111111", providerKeyName: "Main", calls: 7, errors: 0, input: 700, output: 70, cache_read: 0, cache_write: 0, reasoning: 0, cost: 12.5, unpriced: 0 },
  { id: "duo#oldfinger", provider: "duo", providerKeyId: "oldfinger", providerKeyName: "Spare", calls: 2, errors: 0, input: 200, output: 20, cache_read: 0, cache_write: 0, reasoning: 0, cost: 0.5, unpriced: 1 },
];
const cards = (used) => [
  { provider: "duo", name: "Duo", icon: "generic", user: "Main", windows: [{ name: "5h", used, limit: 100 }] },
  { provider: "duo", name: "Duo", icon: "generic", user: "Spare", windows: [{ name: "5h", used: 20, limit: 100 }] },
];

function serve(lang, ctl) {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:${JSON.stringify(lang)},theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/providers") return json({ providers: [duo()], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === "/api/usage") return json({ period: "today", since: new Date().toISOString(), calls: 9, providerKeys: keyGroups(), accounts: [], agents: [], models: [], sessions: [] });
    if (url.pathname === "/api/usage/quotas") {
      ctl.quotasReads++;
      return json(cards(ctl.used));
    }
    if (url.pathname === "/api/usage/quotas/refresh") {
      ctl.asked.push(url.searchParams.get("provider") + " " + url.searchParams.get("user"));
      ctl.used = 80;
      return json([{ provider: "duo", name: "Duo", icon: "generic", user: "Main", windows: [{ name: "5h", used: 80, limit: 100 }] }]);
    }
    if (url.pathname === "/api/gateway/trace") return json({ mine: true, now: new Date().toISOString(), seq: 1, totals: { requests: 0, rerouted: 0, errors: 0 }, routes: [] });
    if (url.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file).catch(() => ""), contentType });
  };
}

// what a string says in lang, its {names} filled in
const say = (page, lang, key, vars = {}) => page.evaluate(([lang, key, vars]) => ((lang !== "en" && I18N[lang]?.[key]) || key).replace(/\{(\w+)\}/g, (_, k) => vars[k] ?? `{${k}}`), [lang, key, vars]);

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: the editor reads no vendor, and a key's day draws only from the local log`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
      const ctl = { used: 64, asked: [], quotasReads: 0 };
      const context = await browser.newContext({ viewport: { width: 1000, height: 800 }, reducedMotion: "reduce" });
      const page = await context.newPage();
      t.after(() => context.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, ctl));
      await page.goto("http://magpie.test/?view=providers");
      await page.locator(".row.provider", { hasText: "Duo" }).first().click();
      const row = (id) => page.locator(`.editor .accts.keys .acc[data-account-id="${id}"]`);
      await row("aaaa111111").locator(".key-usage").waitFor();

      // the day, drawn right: calls, cost, tokens in and out, the tooltip
      // saying it is the Usage page's Today
      const want = (await say(page, lang, "Today {n} calls", { n: 7 })) + " · $12.50 · " + (await say(page, lang, "{a} in · {b} out", { a: "700", b: "70" }));
      assert.equal(await row("aaaa111111").locator(".key-usage span").textContent(), want);
      assert.equal(await row("aaaa111111").locator(".key-usage span").getAttribute("title"), await say(page, lang, "What this key was asked today, as the Usage page's Today counts it"));

      // Spare's records keep an older fingerprint: its day is found by name
      const spare = (await say(page, lang, "Today {n} calls", { n: 2 })) + " · $0.500+ · " + (await say(page, lang, "{a} in · {b} out", { a: "200", b: "20" }));
      assert.equal(await row("bbbb222222").locator(".key-usage span").textContent(), spare);

      // the unnamed key has no day: no line of its own
      assert.equal(await row("cccc333333").locator(".aq").count(), 0, "a key with no calls draws no line");

      // and no card anywhere: the allowances were never read, and the
      // editor asked no vendor for them (a settle, for a read that must
      // never come)
      assert.equal(await page.locator(".editor .accts.keys .acc .aq:not(.key-usage)").count(), 0, "no card before a read");
      await page.waitForTimeout(1200);
      assert.equal(ctl.quotasReads, 0, "the editor asked for the allowances");
      assert.deepEqual(errors, []);
      await context.close();
    }
  });

  test(`${engine}: a key's card is drawn once the routing view has read it, and its refresh reads that card alone`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
      const ctl = { used: 64, asked: [], quotasReads: 0 };
      const context = await browser.newContext({ viewport: { width: 1000, height: 800 }, reducedMotion: "reduce" });
      const page = await context.newPage();
      t.after(() => context.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, ctl));
      // the routing view reads the allowances as it opens (never asked):
      // after that the editor draws the key's card from what was read
      const quotasRead = page.waitForRequest((r) => new URL(r.url()).pathname === "/api/usage/quotas", { timeout: 10000 });
      await page.goto("http://magpie.test/?view=routing");
      await quotasRead;
      assert.ok(ctl.quotasReads > 0, "the routing view read the allowances");
      await page.locator("#nav button[data-view=\"providers\"]").click();
      await page.locator(".row.provider", { hasText: "Duo" }).first().click();
      const row = (id) => page.locator(`.editor .accts.keys .acc[data-account-id="${id}"]`);
      await row("aaaa111111").locator(".key-usage").waitFor();
      const card = row("aaaa111111").locator(".aq:not(.key-usage)");
      await card.locator(".aq-w").waitFor();

      // Main's card: the window magpie read, its tooltip saying the card is
      // only what magpie has read
      assert.ok((await card.locator(".aq-w b").textContent()).includes("64"), "Main's card shows its window");
      assert.equal(await card.getAttribute("title"), await say(page, lang, "This key's allowance, as magpie has read it; refresh asks for this card alone"));
      // Spare's card is there, the unnamed key's is not
      assert.ok((await row("bbbb222222").locator(".aq:not(.key-usage) .aq-w b").textContent()).includes("20"), "Spare's card");
      assert.equal(await row("cccc333333").locator(".aq").count(), 0, "no card for a key none read");

      // the refresh reads this card alone: only Main's is asked, by its
      // provider and user, and the line shows the new reading; Spare's
      // stands as it was read
      await row("aaaa111111").locator(".aq > .quota-refresh").click();
      await page.waitForFunction(() => document.querySelector(".editor .acc[data-account-id=\"aaaa111111\"] .aq:not(.key-usage) .aq-w b")?.textContent.includes("80"));
      assert.deepEqual(ctl.asked, ["duo Main"], "only Main's card was read again");
      await page.waitForFunction(() => !document.querySelector(".editor .quota-refresh.busy"));
      assert.ok((await row("bbbb222222").locator(".aq:not(.key-usage) .aq-w b").textContent()).includes("20"), "Spare's card stands");
      assert.deepEqual(errors, []);
      await context.close();
    }
  });
}
