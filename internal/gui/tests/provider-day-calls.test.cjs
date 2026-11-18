// Run with Node's test runner and Playwright on the module path; see README.md.
// A provider's daily call limit is a provider-level budget, all keys and
// accounts together, on this computer's calendar day. Its editor has the
// field under Advanced, empty for no limit; Save posts the number typed
// (0 for no limit), and refuses one that isn't a whole number from 0 to
// 100000 before anything is posted. The four languages have their words.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings: allStrings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const unfold = async (page) => {
  const adv = page.locator(".editor details.advanced");
  await adv.waitFor({ state: "attached" });
  if (!(await adv.evaluate((d) => d.open))) await adv.locator(":scope > summary").click();
};
const relay = {
  id: "relay", name: "Relay", icon: "generic", chat: "https://relay.example.com/v1", responses: "", anthropic: "", catalog: "",
  models: [{ id: "model-a", name: "Model A", on: true }], agents: [], fallback: [], headers: {},
  key: { set: true, masked: "sk-…one" }, keyList: [], balanceToken: { takes: false, set: false }, proxy: "", maxConcurrency: null,
  dayCalls: 200,
};

function serve(lang, posts) {
  const providers = { providers: [relay], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/provider/") && route.request().method() === "POST") {
      posts.push({ action: url.pathname.slice("/api/provider/".length), body: route.request().postDataJSON() });
      return json(providers);
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const words = {
  en: {
    label: "Daily calls", save: "Save", none: "No limit",
    hint: "How many requests the provider as a whole may take in one day, on this computer's clock; one more is turned away until the next midnight. 0 or empty is no limit",
    bad: "Daily calls: a whole number from 0 to 100000",
  },
  zh: {
    label: "每日调用上限", save: "保存", none: "不限制",
    hint: "这个渠道每天最多可以接多少次请求（按本机时钟的自然日，所有密钥和账号合计）；超出的请求会被拒绝，直到次日 00:00。0 或留空为不限制",
    bad: "每日调用上限：0 到 100000 的整数",
  },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = words[lang];
    const open = async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(async () => {
        if (process.env.ARTIFACT_DIR) {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `${engine}-${lang}-day-calls.png`) });
        }
        await browser.close();
      });
      const page = await (await browser.newContext({ viewport: { width: 900, height: 760 }, reducedMotion: "reduce", colorScheme: "light" })).newPage();
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const posts = [];
      await page.route("**/*", serve(lang, posts));
      await page.goto("http://magpie.test/?view=providers");
      await page.locator(".row.provider", { hasText: "Relay" }).click();
      await unfold(page);
      await page.locator(".editor input.day-calls").waitFor();
      await page.waitForTimeout(300);
      return { page, errors, posts };
    };
    const box = (page) => page.locator(".editor input.day-calls");
    const save = async (page, posts) => {
      const n = posts.length;
      await page.locator(".editor .bar").getByRole("button", { name: w.save, exact: true }).click();
      for (let i = 0; i < 50 && posts.length === n; i++) await page.waitForTimeout(50);
      return posts.at(-1);
    };

    test(`${engine} ${lang}: a provider's daily call limit is typed and saved`, async (t) => {
      const { page, errors, posts } = await open(t);
      assert.equal(await page.locator(".editor label", { hasText: w.label }).count(), 1);
      assert.equal(await box(page).inputValue(), "200", "the saved limit opens in the field");
      assert.equal(await box(page).getAttribute("placeholder"), w.none);
      assert.equal(await box(page).evaluate((e) => e.parentElement.querySelector(".hint")?.textContent), w.hint);

      // not a whole number: refused, nothing posted
      await box(page).fill("2.5");
      await page.locator(".editor .bar").getByRole("button", { name: w.save, exact: true }).click();
      await page.locator(".editor .editor-error").waitFor();
      assert.equal(await page.locator(".editor .editor-error").textContent(), w.bad);
      assert.equal(posts.length, 0);

      // past the bound: refused too
      await box(page).fill("100001");
      await page.locator(".editor .bar").getByRole("button", { name: w.save, exact: true }).click();
      await page.waitForTimeout(100);
      assert.equal(await page.locator(".editor .editor-error").textContent(), w.bad);
      assert.equal(posts.length, 0);

      await box(page).fill("500");
      let saved = await save(page, posts);
      assert.equal(saved.action, "save");
      assert.equal(saved.body.id, "relay");
      assert.equal(saved.body.dayCalls, 500);

      // emptied: no limit
      await page.locator(".row.provider", { hasText: "Relay" }).click();
      await unfold(page);
      await box(page).fill("");
      saved = await save(page, posts);
      assert(Object.hasOwn(saved.body, "dayCalls"), "an empty field is saved as none, not left out");
      assert.equal(saved.body.dayCalls, 0, "0 is no limit");

      assert.deepEqual(errors, []);
    });

    test(`${engine} ${lang}: every language has the daily call words`, async () => {
      const all = allStrings();
      // the words are keyed by the English app.js writes, whatever the page's language
      for (const code of ["zh", "zh-TW", "ja", "de"]) {
        for (const key of [words.en.label, words.en.hint, words.en.bad]) {
          assert(all.I18N[code]?.[key], `${code} is missing ${JSON.stringify(key)}`);
        }
      }
    });
  }
}
