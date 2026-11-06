// Run with Node's test runner and Playwright on the module path; see README.md.
// A subscription account the gateway passes over after a failure (moonshot-cn
// suspended for balance, a ZCode account with no plan left) says why and until
// when on its row in the provider's accounts, with "Try now" beside it, as a
// key's row has since #1869. A click posts gateway/unrest with the account's
// own rest key (provider@user), the note goes, the status says the account is
// asked again, and neither the click nor the row pushes the page sideways. An
// account not resting offers nothing. In every language, at a wide and a
// narrow window, on Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const until = new Date(Date.now() + 23 * 60 * 60e3).toISOString();
const codex = () => ({
  id: "codex", name: "Codex", icon: "codex-color", chat: "", responses: "", anthropic: "", catalog: "", routing: "",
  models: [{ id: "gpt-6", name: "GPT-6", on: true }], agents: [], fallback: [], headers: {}, keyList: [],
  account: {
    agent: "codex", agentName: "Codex", user: "team@example.com", plan: "PLUS",
    logins: [
      { user: "team@example.com", plan: "PLUS", active: true, on: true },
      { user: "broke@example.com", plan: "PLUS", on: true, rest: { why: "credit", status: 402, until, by: "credit", failures: 3, key: "codex@broke@example.com" } },
    ],
  },
  proxy: "",
});

function serve(lang, lifted) {
  const state = { agents: [{ id: "codex", name: "Codex", path: "/test/config.toml", fields: [] }], profiles: [], settings: { lang, theme: "light" } };
  const providers = { providers: [codex()], presets: [], excluded: [], gateway: { running: true, window: true } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/login/usage") return json({ "team@example.com": { windows: [{ label: "5h", used: 10, limit: 100 }] } });
    if (url.pathname === "/api/gateway/unrest") {
      lifted.push(JSON.parse(route.request().postData() || "{}"));
      return json({ lifted: true });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

// the app's own words, as t() gives them
const say = (page, lang, key, vars = {}) => page.evaluate(([lang, key, vars]) => ((lang !== "en" && I18N[lang]?.[key]) || key).replace(/\{(\w+)\}/g, (_, k) => vars[k]), [lang, key, vars]);

const posted = async (lifted) => {
  for (let i = 0; i < 60 && !lifted.length; i++) await new Promise((r) => setTimeout(r, 50));
  return lifted;
};
const scrolled = (page) => page.evaluate(() => [window.scrollY, ...[...document.querySelectorAll("*")].filter((e) => e.scrollTop > 0).map((e) => `${e.className}:${e.scrollTop}`)].join(" "));
const sideways = (page) => page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    for (const width of [1100, 420]) {
      test(`${engine} ${lang} ${width}px: a resting account says why and is tried again by hand`, async (t) => {
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        t.after(() => browser.close());
        const context = await browser.newContext({ viewport: { width, height: 800 }, reducedMotion: "reduce" });
        const page = await context.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        const errors = [], lifted = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang, lifted));
        await page.goto("http://magpie.test/?view=providers");
        await page.locator(".row.provider", { hasText: "Codex" }).first().click();
        const row = page.locator('.editor .acc[data-account-id="broke@example.com"]');
        await row.waitFor();
        const note = row.locator(".key-rest");
        await note.waitFor();
        // why it rests, as the Routing page words it, and when it is back
        const why = await say(page, lang, "out of credit");
        assert((await note.textContent()).includes(why), `"${await note.textContent()}" doesn't say ${why}`);
        assert.equal(await page.locator('.editor .acc[data-account-id="team@example.com"] .key-unrest').count(), 0, "an account not resting offers it");
        assert(!(await sideways(page)), "the row scrolls the page sideways");
        const go = row.getByRole("button", { name: await say(page, lang, "Try now"), exact: true });
        await go.waitFor();
        await go.scrollIntoViewIfNeeded();
        const sc = await scrolled(page);
        await go.click();
        assert.deepEqual(await posted(lifted), [{ key: "codex@broke@example.com" }]);
        await row.locator(".key-rest").waitFor({ state: "detached" });
        assert.equal((await page.locator("#status").textContent()).trim(), await say(page, lang, "{key} is asked again from the next request", { key: "broke@example.com" }));
        assert.equal(await scrolled(page), sc, "the click scrolled");
        assert.deepEqual(errors, []);
      });
    }
  }
}
