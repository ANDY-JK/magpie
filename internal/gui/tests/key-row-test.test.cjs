// A key row's Test (next to Switch on and Remove): Test models asked of that
// key alone — the request carries the key's ref, the answer said in the
// status line with the key's name, the marks landing on the models' chips.
// An off key can be tested too, by the same button. The Tests of the models
// field send no ref, as ever: every model with the first ticked key that can
// serve it. No click moves the page, nothing native, no coloured left
// border. English, Chinese, Traditional Chinese, Japanese and German.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { allWords } = require("./strings.cjs");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const keys = [
  "Test",
  "Test models",
  "All {n} models answered with {key}",
  "Test the models with this key alone; an off key's too, and testing turns nothing on",
];

function serve(lang, posts) {
  const relay = {
    id: "relay", name: "Relay", icon: "generic", chat: "https://relay.example.com/v1", responses: "", anthropic: "", catalog: "",
    models: [{ id: "big", name: "", on: true }, { id: "cheap", name: "Cheap one", on: true }],
    agents: [], fallback: [], headers: {}, key: { set: true, masked: "sk-…one" }, proxy: "",
    keyList: [
      { id: "kp-first", name: "", masked: "sk-…one", on: true, active: true },
      { id: "kp-second", name: "Team", masked: "sk-…two", on: true },
    ],
  };
  const providers = { providers: [relay], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:${JSON.stringify(lang)},theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/provider/test" && route.request().method() === "POST") {
      posts.push(route.request().postDataJSON());
      return json({ results: [{ ok: true, ms: 12 }, { ok: true, ms: 12 }] });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    await route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] });
  };
}

// said is the status line's words for n of n models answered with key
const said = (L, n, key) => L[keys[2]].replaceAll("{n}", String(n)).replace("{key}", key);

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    test(`${engine} ${lang}: a key row's Test asks with that key alone`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 900, height: 720 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, posts));
      await page.goto("http://magpie.test/?view=providers");
      await page.waitForFunction(() => typeof I18N === "object");
      const L = Object.fromEntries(await page.evaluate(([lang, keys]) => keys.map((k) => [k, lang === "en" ? k : I18N[lang][k]]), [lang, keys]));
      for (const k of keys) assert(L[k], `${lang} has ${k}`);
      await page.locator('.row.provider[data-id="relay"]').click();
      const row = (id) => page.locator(`.editor .accts.keys .acc[data-account-id="${id}"]`);
      const btn = (id) => row(id).getByRole("button", { name: L[keys[0]], exact: true });
      const posted = async (n) => {
        for (let i = 0; i < 60 && posts.length === n; i++) await new Promise((r) => setTimeout(r, 50));
        return posts.at(-1);
      };
      // what the page and its scrollers have scrolled
      const scrolled = () => page.evaluate(() => [window.scrollY, ...[...document.querySelectorAll("*")].filter((e) => e.scrollTop > 0).map((e) => `${e.className}:${e.scrollTop}`)].join(" "));
      // the rows have no native select, and the Test says what it does
      assert.equal(await row("kp-second").locator("select").count(), 0);
      assert.equal(await btn("kp-second").getAttribute("title"), L[keys[3]]);
      // Team's Test: the request carries its ref, the models agents see asked
      const before = await row("kp-second").evaluate((e) => Math.round(e.getBoundingClientRect().top)), sc = await scrolled();
      await btn("kp-second").click();
      const byKey = await posted(0);
      assert.equal(byKey.ref, "kp-second", "the request names the key");
      assert.equal(byKey.id, "relay");
      assert.equal(typeof byKey.run, "string");
      assert.deepEqual(byKey.test, ["big", "cheap"]);
      await page.waitForFunction((want) => document.querySelector("#status").textContent === want, said(L, 2, "Team"));
      // a click leaves the row and the page where they were
      await page.evaluate(() => Promise.all(document.querySelector("#modal .dialog").getAnimations().map((a) => a.finished)));
      assert.equal(await row("kp-second").evaluate((e) => Math.round(e.getBoundingClientRect().top)), before, "the row moved");
      assert.equal(await scrolled(), sc, "a click scrolled");
      // the unnamed key's Test asks with its id, and says it by its masked key
      await btn("kp-first").click();
      assert.equal((await posted(1)).ref, "kp-first");
      await page.waitForFunction((want) => document.querySelector("#status").textContent === want, said(L, 2, "sk-…one"));
      // no coloured left border on the rows' controls
      const border = await page.evaluate(() => [...document.querySelectorAll(".key-acts, .key-acts *")].map((e) => getComputedStyle(e).borderLeftWidth).filter((b) => parseFloat(b) > 1));
      assert.deepEqual(border, [], "no border stripes");
      // the models field's Test models sends no ref: the first key that can serve, as ever
      const all = page.getByRole("button", { name: L[keys[1]], exact: true });
      await all.scrollIntoViewIfNeeded();
      const n = posts.length;
      await all.click();
      const whole = await posted(n);
      assert.equal("ref" in whole, false, "the whole test sends no ref");
      assert.deepEqual(whole.test, ["big", "cheap"]);
      await allWords(page); // the other languages' words are files of their own (i18n.js, plugins/<id>/i18n.js)
      const missing = await page.evaluate((keys) => keys.slice(2).flatMap((k) => ["zh", "zh-TW", "ja", "de"].filter((l) => !I18N[l][k]).map((l) => `${l}: ${k}`)), keys);
      assert.deepEqual(missing, [], "every string has every language");
      assert.deepEqual(errors, []);
    });
  }
}
