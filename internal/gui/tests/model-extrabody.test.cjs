// A model can be given extra body fields of its own (the relay that wants
// one field of one model's requests different), in the provider editor's
// Names & levels beside the price: a small JSON box only on a row the
// provider serves, showing what is kept, staged and made with the editor's
// Save as the rest of the row is. An emptied box is none of the model's
// own — its provider's Extra body sends again — and a field magpie sets
// itself, or JSON that isn't an object, is said where it was typed and the
// Save refuses it. No click moves the page, nothing native, no coloured
// left border. English, Chinese, Traditional Chinese, Japanese and German.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { allWords } = require("./strings.cjs");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const levels = ["none", "low", "medium", "high"];

function serve(lang, posts) {
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  const models = [
    { id: "free", name: "free", on: true, efforts: levels, images: false, xb: { who: "kept-1" } },
    { id: "m", name: "m", on: true, efforts: levels, images: false },
  ];
  const relay = { id: "relay", name: "Relay", icon: "generic", chat: "https://relay.example/v1", responses: "", anthropic: "", models, agents: [], key: { set: true, masked: "sk-…1234" }, ready: true };
  const providers = { providers: [relay], presets: [], excluded: [], gateway: { running: true, window: true } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:${JSON.stringify(lang)},theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname.startsWith("/api/provider/")) {
      posts.push({ path: url.pathname, body: route.request().postDataJSON() });
      return json(providers);
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    await route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] });
  };
}

const keys = ["Names & levels", "Extra body", "unsaved", "Save", "Add model"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    test(`${engine} ${lang}: a model's extra body is staged and made with the provider's Save`, async (t) => {
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
      await page.getByRole("button", { name: L["Names & levels"], exact: true }).click();
      const row = (id) => page.locator(".mname", { has: page.locator("code", { hasText: new RegExp(`^${id}$`) }) });
      const box = (id) => row(id).getByRole("textbox", { name: L["Extra body"], exact: true });
      // what is kept shows as JSON; a model with none of its own is empty
      assert.equal(await box("free").inputValue(), JSON.stringify({ who: "kept-1" }));
      assert.equal(await box("m").inputValue(), "");
      // a model added by its id is not on the provider's list: no box
      await page.locator(".mfoot input").fill("hand-made");
      await page.getByRole("button", { name: L["Add model"], exact: true }).click();
      assert.equal(await row("hand-made").locator(".mxb").count(), 0, "no box on a model not served");
      // staged, it is made with the Save; nothing is sent before it
      const list = await page.locator(".mnames:not([hidden])").elementHandle();
      const y = await page.evaluate(() => scrollY);
      await box("m").fill('{"who":"model-1","!service_tier":"flex"}');
      await box("m").press("Enter");
      await box("free").fill("");
      await box("free").press("Enter");
      assert.deepEqual(posts, [], "nothing is sent before the Save");
      assert(await list.evaluate((e) => e.isConnected), "the list is not redrawn");
      assert.equal(await page.evaluate(() => scrollY), y, "nothing scrolls the page");
      for (const id of ["free", "m"]) assert(await row(id).getByText(L.unsaved, { exact: true }).isVisible(), id + " says it is unsaved");
      // the box sits in the row as Same as does, no wider than the row
      const fits = await row("m").evaluate((r) => [...r.querySelectorAll("input")].every((i) => i.getBoundingClientRect().right <= r.getBoundingClientRect().right + 1));
      assert(fits, "the box fits its row");
      // a field magpie sets itself is said where it was typed, and the Save
      // refuses it without posting
      await box("m").fill('{"model":"other"}');
      await box("m").press("Enter");
      await page.getByRole("button", { name: L.Save, exact: true }).click();
      assert.equal(posts.length, 0, "a reserved name is not sent");
      assert(await list.evaluate((e) => e.isConnected), "the Save refuses, the list staying open");
      const raw = row("m").locator(".mxb input");
      const why = await row("m").locator(".mxb .why").textContent();
      assert(why && why !== "", "the box says what is wrong beside it");
      assert.deepEqual(errors, [], "no page error");
      // the Save takes the staged and the emptied ones once it is right
      await raw.fill('{"who":"model-1"}');
      await raw.press("Enter");
      await page.getByRole("button", { name: L.Save, exact: true }).click();
      await page.waitForFunction(() => !document.querySelector(".mnames"));
      assert.deepEqual(posts.map((p) => p.path), ["/api/provider/save"]);
      assert.deepEqual(posts[0].body.modelPrefs.m, { extraBody: { who: "model-1" } });
      assert.deepEqual(posts[0].body.modelPrefs.free, { extraBody: {} }, "an emptied box takes the model's own away");
      assert.equal(posts[0].body.modelPrefs["hand-made"], undefined, "a model not served is saved as nothing");
      await allWords(page); // the other languages' words are files of their own (i18n.js, plugins/<id>/i18n.js)
      const missing = await page.evaluate(() => ["Extra body",
        "Extra body fields of {id}'s own, over the provider's Extra body: a field both give goes out as the model's says it, \"!name\" replacing one the request has; ${sessionId} is the conversation's id. Empty: none of its own",
        "Extra body of {id}: {why}",
      ].flatMap((k) => ["zh", "zh-TW", "ja", "de"].filter((l) => !I18N[l][k]).map((l) => `${l}: ${k}`)));
      assert.deepEqual(missing, [], "every string has every language");
      assert.deepEqual(errors, []);
    });
  }
}
