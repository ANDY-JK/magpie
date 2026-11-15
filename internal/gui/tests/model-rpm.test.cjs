// A model can be given Requests a minute of its own (coeo91 on Discord:
// OpenRouter's free models take 20, and one free model's cap is not its
// neighbour's), in the provider editor's Names & levels beside Wire as: a
// small number box only on a row the provider serves, showing what is
// kept, staged and made with the editor's Save as the rest of the row is.
// An emptied box is none of the model's own — its key or account's
// Requests per minute sends again — and the smaller of the two is the one
// that counts (a limit only ever tightens). A model added by its id, no
// longer on the provider's list, has no box to fill. No click moves the
// page, nothing native, no coloured left border. English, Chinese,
// Traditional Chinese, Japanese and German.
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
    { id: "free", name: "free", on: true, efforts: levels, images: false, rpm: 20 },
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
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/provider/")) {
      posts.push({ path: url.pathname, body: route.request().postDataJSON() });
      return json(providers);
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    await route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)] });
  };
}

const keys = ["Names & levels", "RPM", "No limit", "unsaved", "Save", "Add model"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    test(`${engine} ${lang}: a model's requests a minute is staged and made with the provider's Save`, async (t) => {
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
      const box = (id) => row(id).getByRole("spinbutton", { name: L["RPM"], exact: true });
      // what is kept shows; a model with none of its own has an empty box
      assert.equal(await box("free").inputValue(), "20");
      assert.equal(await box("m").inputValue(), "");
      assert.equal(await box("m").getAttribute("placeholder"), L["No limit"]);
      assert.equal(await box("free").getAttribute("type"), "number", "a number box, not a pick list");
      assert.equal(await row("free").locator("select").count(), 0);
      // a model added by its id is not on the provider's list: no box
      await page.locator(".mfoot input").fill("hand-made");
      await page.getByRole("button", { name: L["Add model"], exact: true }).click();
      assert.equal(await row("hand-made").locator(".mrpm").count(), 0, "no box on a model not served");
      // staged, it is made with the Save; nothing is sent before it
      const list = await page.locator(".mnames:not([hidden])").elementHandle();
      const y = await page.evaluate(() => scrollY);
      await box("m").fill("5");
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
      await page.getByRole("button", { name: L.Save, exact: true }).click();
      await page.waitForFunction(() => !document.querySelector(".mnames"));
      assert.deepEqual(posts.map((p) => p.path), ["/api/provider/save"]);
      assert.deepEqual(posts[0].body.modelPrefs.m, { rpm: 5 });
      assert.deepEqual(posts[0].body.modelPrefs.free, { rpm: 0 }, "an emptied box takes the user's away");
      assert.equal(posts[0].body.modelPrefs["hand-made"], undefined, "a model not served is saved as nothing");
      await allWords(page); // the other languages' words are files of their own (i18n.js, plugins/<id>/i18n.js)
      const missing = await page.evaluate(() => ["RPM",
        "How many requests {id} sends the vendor in any minute, retries included; the smaller of this and the key or account's Requests per minute is the one that counts. Empty: no bound of its own",
      ].flatMap((k) => ["zh", "zh-TW", "ja", "de"].filter((l) => !I18N[l][k]).map((l) => `${l}: ${k}`)));
      assert.deepEqual(missing, [], "every string has every language");
      assert.deepEqual(errors, []);
    });
  }
}
