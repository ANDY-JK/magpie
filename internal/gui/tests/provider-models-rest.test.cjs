// Run with Node's test runner and Playwright on the module path; see README.md.
// A long model list in the provider editor is drawn up to its first 80, but
// a picked model is never left out of it, and the foot of the list is a
// button that draws the rest (#685: Sun1090 picked 10 of OpenCode Zen's 84
// models and saw 7, then "… 4 more, filter to find them" with no names to
// filter by). Opening the rest doesn't move the editor. In English and
// Chinese, Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const fetched = new Date(Date.now() - 3600e3).toISOString();
const ids = Array.from({ length: 84 }, (_, i) => `zen/model-${i}`);
// ten picked, three of them past the first 80
const chosen = ["zen/model-1", "zen/model-4", "zen/model-9", "zen/model-20", "zen/model-33", "zen/model-50", "zen/model-77", "zen/model-80", "zen/model-82", "zen/model-83"];

const provider = {
  id: "zen", name: "Zen", icon: "generic", host: "zen.example.com", chat: "https://zen.example.com/v1", responses: "", anthropic: "", catalog: "",
  models: ids.map((id) => ({ id, name: id, on: chosen.includes(id) })), chosen, fetched, agents: [], fallback: [], headers: {},
  key: { set: true, masked: "sk-…one" }, keyList: [], balanceToken: { takes: false, set: false }, proxy: "",
};

function serve(lang) {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/providers") return json({ providers: [provider], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const rest = { en: "Show the other 74", zh: "显示其余 74 个" };
const last1 = { en: "Show the other 1", zh: "显示其余 1 个" };
// 79 joins the picks on the way back: the picks are 11, and the list's
// first 80 plus every pick leave 73 out
const last73 = { en: "Show the other 73", zh: "显示其余 73 个" };

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    test(`${engine} ${lang}: every picked model is in sight, and the rest are a click away`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 900, height: 760 }, reducedMotion: "reduce" })).newPage();
      t.after(async () => {
        if (process.env.ARTIFACT_DIR) {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `${engine}-${lang}-provider-models-rest.png`) });
        }
        await browser.close();
      });
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang));
      await page.goto("http://magpie.test/?view=providers");
      const editor = page.locator("#modal:not([hidden]) .editor");
      await page.locator(".row.provider", { hasText: "Zen" }).click();
      await editor.locator(".mchips .mchip").first().waitFor();
      const chips = editor.locator(".mchips").first();
      const names = (sel) => chips.locator(sel).evaluateAll((cs) => cs.map((c) => c.firstChild.textContent));

      // Picked only (the serves control, 9b538a3f3) opens on the picks
      // alone: all ten are there, the three past 80 too
      assert.deepEqual(await names(".mchip.on"), chosen);
      assert.equal(await chips.locator(".mchip").count(), 10);
      const more = chips.getByRole("button", { name: rest[lang], exact: true });
      await more.waitFor();

      // the click shows the rest, up to the first 80 plus every pick; a
      // pick past the 80 is drawn, one just past it is left out
      await more.click();
      assert.equal(await chips.locator(".mchip").count(), 83);
      assert.deepEqual(await names(".mchip.on"), chosen);
      const first80 = ids.filter((_, i) => i < 80 || chosen.includes(ids[i]));
      assert.deepEqual(await names(".mchip:not(.on)"), first80.filter((id) => !chosen.includes(id)));

      // a chip clicked while the rest are in sight keeps every model: the
      // click sets unpickedShown, so the picks-only view draws them all
      // (model-81, left out by the first click, comes back behind a button)
      const past = chips.locator(".mchip", { hasText: /^zen\/model-79$/ });
      await past.scrollIntoViewIfNeeded();
      await past.click();
      assert.equal(await chips.locator(".mchip").count(), 83);
      assert.ok((await names(".mchip.on")).includes("zen/model-79"));
      const more1 = chips.getByRole("button", { name: last1[lang], exact: true });
      await more1.waitFor();
      await more1.click();
      assert.equal(await chips.locator(".mchip").count(), 84);
      // the new pick joins the order it was drawn in: the list's own, then
      // the click's, and the ones past 80 after their unpicked neighbours
      assert.deepEqual(await names(".mchip.on"), [...chosen.slice(0, 7), "zen/model-79", ...chosen.slice(7)]);

      // back on the picks: the button folds the rest away again, and the
      // click draws the same first 80 plus every pick
      const seg = (v) => editor.locator(`.mserves .opt[data-serves=${v}]`);
      await seg("list").click();
      await seg("picked").click();
      const folded = chips.getByRole("button", { name: last73[lang], exact: true });
      await folded.waitFor();
      assert.equal(await chips.locator(".mchip").count(), 11);
      assert.deepEqual(await names(".mchip.on"), [...chosen.slice(0, 7), "zen/model-79", ...chosen.slice(7)], "the picks-only view holds every pick");
      await folded.click();
      await page.waitForTimeout(300);
      assert.equal(await chips.locator(".mchip").count(), 84);
      assert.deepEqual(await names(".mchip.on"), [...chosen.slice(0, 7), "zen/model-79", ...chosen.slice(7)], "opening the rest picks nothing");
      assert.deepEqual(errors, []);
    });
  }
}
