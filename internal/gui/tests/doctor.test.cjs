// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 出了问题要排查 — the Doctor page says who is resting and
// why (a key refused, out of credit, rate limited), what failed lately by
// provider and why, and offers Try now for a rest (the same gateway/unrest a
// provider row's button posts) and Open provider. Nothing is changed by the
// page itself. English and Chinese, Chromium and WebKit; no backend, the API
// is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const soon = (mins) => new Date(Date.now() + mins * 60000).toISOString();
const relay = {
  id: "relay", name: "Relay", icon: "generic", chat: "", responses: "", anthropic: "", catalog: "",
  models: [], agents: [], fallback: [], headers: {}, key: { set: true, masked: "sk-…one" },
  keyList: [
    { id: "k1", name: "Main", masked: "sk-…1", on: true },
    { id: "k2", name: "Backup", masked: "sk-…2", on: true, rest: { why: "rate", status: 429, until: soon(20), key: "relay#k2" } },
    { id: "k3", name: "Dead", masked: "sk-…3", on: true, rest: { why: "auth", status: 401, until: soon(24 * 60), key: "relay#k3" } },
  ],
  balanceToken: { takes: false, set: false }, proxy: "",
};
const acct = {
  id: "acct", name: "Account", icon: "generic", chat: "", responses: "", anthropic: "", catalog: "",
  models: [], agents: [], fallback: [], headers: {}, key: { set: false }, keyList: [],
  account: { agent: "codex", agentName: "Codex", user: "me@example.com", logins: [
    { user: "me@example.com", alias: "Work", plan: "PLUS", active: true, on: true, rest: { why: "quota", status: 429, until: soon(120), key: "codex@work" } },
  ] },
  balanceToken: { takes: false, set: false }, proxy: "",
};
const now = Date.now();
const ROWS = [
  { t: new Date(now - 60e3).toISOString(), provider: "relay", providerName: "Relay", model: "gpt-6", status: 429, err_type: "rate_limit", err: "slow down" },
  { t: new Date(now - 120e3).toISOString(), provider: "relay", providerName: "Relay", model: "gpt-6", status: 429, err_type: "rate_limit", err: "slow down" },
  { t: new Date(now - 180e3).toISOString(), provider: "acct", providerName: "Account", model: "gpt-6", status: 401, err_type: "auth_error", err: "bad key" },
];

function server(lang, state, posts) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/providers") return json({ providers: [relay, acct], presets: [], excluded: [], gateway: { running: true } });
    if (url.pathname === "/api/usage/requests") return json({ rows: ROWS, total: ROWS.length, offset: 0 });
    if (url.pathname === "/api/gateway/unrest") { posts.push(req.postDataJSON()); return json({}); }
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const L = {
  en: { title: "Doctor", sub: "Who is resting, why, and what failed lately", rest: "Resting now", fail: "Failed lately", try: "Try now", open: "Open provider", asked: "It is asked again now", none: "Nothing is resting now.", nofail: "Nothing failed lately." },
  zh: { title: "医生", sub: "谁在休息、为什么，以及最近哪些请求失败", rest: "正在休息", fail: "最近的失败", try: "Try now", open: "打开供应商", asked: "已立刻再问它一次", none: "现在没有 key 或账号在休息。", nofail: "最近没有失败的请求。" },
};
const KEYS = ["Read again", "Resting now", "Failed lately", "Nothing is resting now.", "Nothing failed lately.",
  "Resting is magpie passing a key or account over until its time. Try now asks it again at once; refused again, it rests again.",
  "providers on", "resting", "failed lately", "{n} keys and accounts", "{n} requests", "It is asked again now", "Open provider", "Read {time}"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the Doctor page says who rests, why, and what failed`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1000, height: 820 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", server(lang, {}, posts));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="doctor"]').click();
      await page.locator("#view-doctor").waitFor();
      await page.waitForFunction((want) => document.querySelector("#drRestCount")?.textContent.includes("3"), 3);
      // what the page is, and the counts
      assert.equal((await page.locator("#drTitle").textContent()).trim(), w.title);
      assert.equal((await page.locator("#drSub").textContent()).trim(), w.sub);
      const cards = await page.locator("#drCards .dr-card b").allTextContents();
      assert.deepEqual(cards, ["2", "3", "3"], "providers on, resting, failed");
      assert.equal((await page.locator("#drRestHead").textContent()).trim(), w.rest);
      assert.equal((await page.locator("#drFailHead").textContent()).trim(), w.fail);
      // each resting key or account, its why and when
      const rests = page.locator("#drRests .dr-rest");
      assert.equal(await rests.count(), 3);
      const subs = await rests.locator(".sub").allTextContents();
      assert.deepEqual(subs, ["Backup · " + (lang === "zh" ? "密钥" : "Key"), "Dead · " + (lang === "zh" ? "密钥" : "Key"), "Work · " + (lang === "zh" ? "账号" : "account")]);
      const why = await rests.locator(".dr-why").allTextContents();
      assert(why[0].includes("429") && why[0].length > 4, why[0]);
      assert(why[1].includes("401"), why[1]);
      assert(why[2].includes("429"), why[2]);
      // a key refused outright is marked bad
      assert.equal(await rests.nth(1).evaluate((e) => e.classList.contains("bad")), true);
      // the failed requests, grouped, the most frequent first
      const fails = page.locator("#drFails .dr-fail");
      assert.equal(await fails.count(), 2);
      assert.equal((await fails.nth(0).locator(".name").textContent()).trim(), "Relay");
      assert.equal((await fails.nth(0).locator(".dr-why").textContent()).trim(), "rate_limit");
      assert.equal((await fails.nth(0).locator(".dr-count").textContent()).trim(), "2×");
      assert.equal((await fails.nth(1).locator(".name").textContent()).trim(), "Account");
      // Try now ends the rest, as the provider's button does
      await rests.nth(0).locator(".dr-try").click();
      for (let i = 0; i < 60 && !posts.length; i++) await page.waitForTimeout(50);
      assert.deepEqual(posts, [{ key: "relay#k2" }]);
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.asked);
      // Open provider goes to it
      await page.locator("#drFails .dr-fail").first().locator(".dr-open").click();
      await page.waitForFunction(() => !document.querySelector("#view-providers")?.hidden);
      const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every new word has its Chinese");
      assert.deepEqual(errors, []);
    });
  }
}
