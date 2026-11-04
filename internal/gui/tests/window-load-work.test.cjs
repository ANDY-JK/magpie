const assert = require("node:assert/strict");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { fixture } = require("./fixtures/caller-keys.cjs");

// A window reads its settings again every time it comes back (loadBack), and
// the settings carry the language it already shows. Rewriting every one of
// the window's own words for the same language, and re-fitting the header
// after it, was work each return paid for nothing (kernel.js still translates
// a page's markup as it is added). ago() made a RelativeTimeFormat for every
// row it drew, where one per language serves them all.
for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: a return to the window rewrites none of its words, and ago() makes one format`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    const page = await (await browser.newContext({ viewport: { width: 1100, height: 800 }, reducedMotion: "reduce" })).newPage();
    page.setDefaultTimeout(8000);
    page.setDefaultNavigationTimeout(60000);
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.addInitScript(() => {
      // writes to a static word, the ones translateStatic makes
      window.__tWrites = 0;
      const d = Object.getOwnPropertyDescriptor(Node.prototype, "textContent");
      Object.defineProperty(Node.prototype, "textContent", {
        configurable: true,
        get() { return d.get.call(this); },
        set(v) { if (this.nodeType === 1 && this.hasAttribute("data-en")) window.__tWrites++; d.set.call(this, v); },
      });
      // every RelativeTimeFormat the page makes
      window.__rtf = 0;
      const RTF = Intl.RelativeTimeFormat;
      Intl.RelativeTimeFormat = function (...a) { window.__rtf++; return new RTF(...a); };
      Intl.RelativeTimeFormat.prototype = RTF.prototype;
    });
    let states = 0;
    page.on("response", (r) => { if (new URL(r.url()).pathname === "/api/state") states++; });
    await page.route("**/*", fixture("zh", "light", []));
    await page.goto("http://magpie.test/");
    await page.locator('#nav button[data-view="providers"]').waitFor();
    await page.waitForFunction(() => document.querySelectorAll("[data-t][data-en]").length > 50);

    const made = await page.evaluate(() => {
      window.__rtf = 0;
      for (let i = 0; i < 25; i++) ago(new Date(Date.now() - i * 3600e3));
      return window.__rtf;
    });
    assert.ok(made <= 1, `ago() made ${made} RelativeTimeFormats for 25 rows, want at most 1 for the language`);

    // the window comes back: one settings read, and no rewrite of its words
    await page.waitForTimeout(1200);
    await page.evaluate(() => { window.__tWrites = 0; });
    const was = states;
    await page.evaluate(() => {
      document.dispatchEvent(new Event("visibilitychange"));
      window.dispatchEvent(new Event("focus"));
    });
    for (let i = 0; i < 40 && states === was; i++) await page.waitForTimeout(100);
    assert.ok(states > was, "the window asked for its state again when it came back");
    await page.waitForTimeout(500);
    const writes = await page.evaluate(() => window.__tWrites);
    assert.ok(writes < 20, `a return to the window in its own language rewrote ${writes} of its words, want none`);
    assert.deepEqual(errors, []);
  });
}
