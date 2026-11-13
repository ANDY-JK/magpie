// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): a key that stopped answering is found on the Providers
// page, not only inside the provider editor. A provider's row now says how
// many of its keys or accounts the gateway is passing over — "1 failed" in
// red for one refused outright (a sign-in that ran out, an account out of
// credit), "1 resting" in amber for a limit — and each one's name, why and
// back-at time on hover. A provider with none says nothing. Every language,
// at 1100 and 420 wide: no sideways scroll. No backend: the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const soon = (mins) => new Date(Date.now() + mins * 60000).toISOString();
const base = {
  icon: "generic", chat: "http://127.0.0.1:8317/v1", responses: "", anthropic: "", catalog: "",
  models: [{ id: "glm-5", name: "glm-5", on: true }], agents: [], fallback: [], headers: {},
  balanceToken: { takes: false, set: false }, proxy: "", searches: false,
};
const key = (id, name, masked, rest) => ({ id, name, masked, on: true, active: true, weight: 0, plan: "", key: masked, ...(rest ? { rest } : {}) });
const agg = {
  ...base, id: "agg", name: "Agg", key: { set: true, masked: "sk-…aaa" },
  keyList: [
    key("k1", "Main", "sk-…aaa"),
    key("k2", "Backup", "sk-…bbb", { why: "auth", status: 401, until: soon(24 * 60), key: "agg#k2", probed: false }),
    key("k3", "Spare", "sk-…ccc", { why: "rate", status: 429, until: soon(20), key: "agg#k3", probed: false }),
  ],
};
const clean = { ...base, id: "clean", name: "Clean Relay", chat: "http://127.0.0.1:8318/v1", key: { set: true, masked: "sk-…ok" }, keyList: [key("k1", "Only", "sk-…ok")] };
const codex = {
  id: "codex", name: "Codex", icon: "codex-color", chat: "", responses: "", anthropic: "", catalog: "",
  models: [{ id: "gpt-6", name: "GPT-6", on: true }], agents: [], fallback: [], headers: {}, keyList: [], proxy: "",
  account: { agent: "codex", agentName: "Codex", user: "me@example.com", plan: "PLUS", logins: [
    { user: "me@example.com", alias: "Work", plan: "PLUS", active: true, on: true, rest: { why: "credit", status: 402, until: soon(60), key: "codex@me@example.com" } },
  ] },
};
// a key the vendor refuses outright: a 401 that says nothing else, its why
// "other" and by "key" (gateway's keyRefusedRest), skipped for the day
const voided = { ...base, id: "voided", name: "Voided", chat: "http://127.0.0.1:8319/v1", key: { set: true, masked: "sk-…vvv" }, keyList: [
  key("k1", "Dead", "sk-…vvv", { why: "other", status: 401, until: soon(24 * 60), key: "voided#k1", by: "key", probed: false }),
] };
// a provider its vendor's firewall blocks as a whole: the rest is the
// provider's own (gateway's blockedKey, id+"*"), no key's or account's
const walled = {
  ...base, id: "walled", name: "Walled", chat: "http://127.0.0.1:8320/v1", key: { set: true, masked: "sk-…www" },
  keyList: [key("k1", "Only", "sk-…www")],
  rest: { why: "other", status: 403, until: soon(24 * 60), key: "walled*", by: "blocked" },
};

let I18N;
const loadI18N = () => { I18N = strings().I18N; };
function tr(lang, key, vars = {}) {
  let s = key;
  if (lang !== "en") {
    assert.equal(typeof I18N[lang][key], "string", `${lang} has no ${JSON.stringify(key)}`);
    s = I18N[lang][key];
  }
  return s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
}

function serve(lang) {
  const providers = { providers: [agg, clean, codex, voided, walled], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a provider row says which key or account is resting", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    loadI18N();
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
      await t.test(lang, async () => {
        const w = (key, vars) => tr(lang, key, vars);
        for (const width of [1100, 420]) {
          const ctx = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: "reduce" });
          const page = await ctx.newPage();
          page.setDefaultTimeout(5000);
          page.setDefaultNavigationTimeout(60000);
          const errors = [];
          page.on("pageerror", (e) => errors.push(e.message));
          await page.route("**/*", serve(lang));
          await page.goto("http://magpie.test/");
          await page.locator('button[data-view="providers"]').click();
          const row = (name) => page.locator("#providers .row.provider").filter({ has: page.locator(".name", { hasText: name }) }).first();
          await row("Agg").waitFor();
          const badges = row("Agg").locator(".name .badge.rest-now");
          assert.equal(await badges.count(), 2, "the failed and the resting badge");
          assert.equal((await badges.nth(0).textContent()).trim(), w("{n} failed", { n: 1 }));
          assert.equal((await badges.nth(1).textContent()).trim(), w("{n} resting", { n: 1 }));
          // each one's name, why and back-at time on hover
          const title = await badges.nth(0).getAttribute("title");
          assert(title.includes("Backup"), `the title names the key: ${title}`);
          assert(title.includes("Spare"), `the title names the resting key: ${title}`);
          assert(title.includes(w("sign-in required")), `the title says why: ${title}`);
          assert(title.includes(w("rate limited")), `the title says why: ${title}`);
          assert(title.split("\n").length === 2, `one line a key: ${title}`);
          // a provider with none says nothing
          assert.equal(await row("Clean Relay").locator(".badge.rest-now").count(), 0);
          // an account out of credit is a failure too, named by its alias
          const cb = row("Codex").locator(".name .badge.rest-now.bad");
          assert.equal(await cb.count(), 1);
          assert.equal((await cb.textContent()).trim(), w("{n} failed", { n: 1 }));
          assert((await cb.getAttribute("title")).includes("Work"), "the account's alias");
          // a key the vendor refuses outright is red, not resting, and its
          // hover says the key was refused (by "key", not a limit)
          const vk = row("Voided").locator(".name .badge.rest-now.bad");
          assert.equal(await vk.count(), 1, "a refused key is a failure");
          assert.equal((await vk.textContent()).trim(), w("{n} failed", { n: 1 }));
          assert.equal(await row("Voided").locator(".name .badge.rest-now:not(.bad)").count(), 0, "and not also resting");
          const vtitle = await vk.getAttribute("title");
          assert(vtitle.includes(w("key refused")), `the hover says the key was refused: ${vtitle}`);
          // a provider its vendor blocks as a whole says so on its own row,
          // in a sentence of its own rather than a key's
          const wb = row("Walled").locator(".name .badge.rest-now");
          assert.equal(await wb.count(), 1, "the whole block is on the provider's row");
          assert.equal((await wb.textContent()).trim(), w("{n} resting", { n: 1 }));
          const wtitle = await wb.getAttribute("title");
          assert(wtitle.startsWith("Walled · "), `the whole block is the provider's own: ${wtitle}`);
          const whole = w("Blocked as a whole: the vendor's firewall refuses this computer's address, or its endpoint answers its web page · back at {time}", { time: "\u0000" });
          for (const piece of whole.split("\u0000").filter(Boolean)) assert(wtitle.includes(piece), `the hover says the provider is blocked as a whole: ${wtitle}`);
          // nothing wider than the window
          const over = await page.evaluate(() => {
            const w = document.documentElement.clientWidth, out = [];
            if (document.documentElement.scrollWidth > w) out.push("page " + document.documentElement.scrollWidth);
            const v = document.querySelector("#view-providers");
            if (v.scrollWidth > v.clientWidth + 1) out.push("providers " + v.scrollWidth + ">" + v.clientWidth);
            return out;
          });
          assert.deepEqual(over, [], `sideways scroll at ${width}`);
          assert.deepEqual(errors, []);
          await ctx.close();
        }
      });
    }
  });
}
