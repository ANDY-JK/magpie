// Run with Node's test runner and Playwright on the module path; see README.md.
// The page parses the words of the language it opens in, and only them:
// i18n.js is the core (locale, t, the notices) and each language is a file
// of its own (i18n.zh.js and the rest), written into the page as it is
// parsed. An English page fetches none of them; a Chinese one fetches
// i18n.zh.js alone. The language is the one setLocale picks: ?locale= in
// the URL, then the saved one (boot.js), then the system's.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { I18N } = require("./strings.cjs").strings(); // each language file's words

const assets = path.resolve(__dirname, "../assets");
const LANG_FILES = ["i18n.zh.js", "i18n.zh-TW.js", "i18n.ja.js", "i18n.de.js"];
const LANG_TAGS = { en: "en", zh: "zh-CN", "zh-TW": "zh-TW", ja: "ja-JP", de: "de-DE" };

function serve(lang) {
  const settings = { lang, theme: "light" };
  return async (r) => {
    const url = new URL(r.request().url());
    const json = (data) => r.fulfill({ json: data });
    if (url.pathname === "/boot.js") return r.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:${JSON.stringify(lang)},theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return r.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings });
    if (url.pathname === "/api/settings") return json(settings);
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true, url: "http://127.0.0.1:3999" } });
    if (url.pathname === "/api/groups") return json({ models: [], groups: [], pools: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await r.fulfill({ body: await fs.readFile(file), contentType });
  };
}

// lang is what the saved settings say; query is taken off the page's URL;
// system is the browser's own language.
const cases = [
  ["English parses no language at all", "en", "", "en-US", "en"],
  ["Chinese parses i18n.zh.js alone", "zh", "", "en-US", "zh"],
  ["?locale= wins over the saved language", "en", "?locale=zh-TW", "en-US", "zh-TW"],
  ['the system\'s language when the saved one is "system"', "system", "", "ja-JP", "ja"],
];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const [name, lang, query, system, want] of cases) {
    test(`${engine}: ${name}`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ locale: system, viewport: { width: 1000, height: 760 } })).newPage();
      page.setDefaultTimeout(8000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const asked = [];
      page.on("request", (r) => {
        const u = new URL(r.url()).pathname.slice(1);
        if (LANG_FILES.includes(u)) asked.push(u);
      });
      await page.route("**/*", serve(lang));
      await page.goto("http://magpie.test/" + query);
      await page.waitForFunction(() => typeof locale === "string" && document.documentElement.lang);
      const got = await page.evaluate(() => ({
        locale, lang: document.documentElement.lang, word: t("Scroll to top"),
        // the languages whose core words the page holds
        core: ["zh", "zh-TW", "ja", "de"].filter((l) => I18N[l]?.["Scroll to top"] !== undefined),
      }));
      // the page's language, chosen as setLocale chooses it
      assert.equal(got.locale, want);
      assert.equal(got.lang, LANG_TAGS[want]);
      assert.equal(got.word, want === "en" ? "Scroll to top" : I18N[want]["Scroll to top"]);
      // and only that language's file was fetched: English fetches none
      assert.deepEqual([...new Set(asked)].sort(), want === "en" ? [] : [`i18n.${want}.js`]);
      // it holds that language's words, and no other's: English holds none
      assert.deepEqual(got.core, want === "en" ? [] : [want], "the languages the page holds");
      assert.deepEqual(await page.evaluate(() => [...localeWordsHere]), want === "en" ? [] : [want], "the language files loaded");
      assert.deepEqual(errors, []);
    });
  }
}
