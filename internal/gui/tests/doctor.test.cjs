// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 出了问题要排查 — the Doctor page says who is resting and
// why (a key refused, out of credit, rate limited, restricted by the vendor),
// names the key or account that failed lately, lists what is signed out,
// paused or off, and offers Try now (the same gateway/unrest a provider row's
// button posts), Probe and recover (gateway/probe, which asks the vendor
// first) and Try all, a fifth of a second apart. The page reads itself again
// every 45 seconds while it is in sight. Nothing is changed by the page
// itself. English and Chinese, Chromium and WebKit; no backend, the API is
// faked.
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
    { id: "k4", name: "Spare", masked: "sk-…4", on: false },
    { id: "k5", name: "Locked", masked: "sk-…5", on: true, rest: { why: "restricted", status: 403, until: soon(600), key: "relay#k5", error: "This account was restricted" } },
  ],
  balanceToken: { takes: false, set: false }, proxy: "",
};
const CATS = [
  { app: "DuMate", provider: "relay", source: "C:/Users/me/AppData/Roaming/qianfan-desktop-app/qianfan_desk_xdg/acct/config/opencode/opencode.json", count: 3,
    models: [{ id: "model-text" }, { id: "deepseek-v41-flash" }, { id: "qwen2.5-vl-32b-instruct" }] },
  { app: "TeleAgent", provider: "teleagent", source: "C:/Users/me/.config/TeleAgent/users/v1_public_x/TeleAgent.jsonc", count: 5,
    models: [{ id: "chat-pro" }, { id: "chat-flagship" }, { id: "chat-flash" }, { id: "chat-lite" }, { id: "chat-nano" }] },
];
const acct = {
  id: "acct", name: "Account", icon: "generic", chat: "", responses: "", anthropic: "", catalog: "",
  models: [], agents: [], fallback: [], headers: {}, key: { set: false }, keyList: [],
  account: { agent: "codex", agentName: "Codex", user: "me@example.com", logins: [
    { user: "me@example.com", alias: "Work", plan: "PLUS", active: true, on: true, rest: { why: "quota", status: 429, until: soon(120), key: "codex@work" } },
    { user: "old@example.com", alias: "Old", active: false, on: false, lapsed: "Sign-in has lapsed" },
    { user: "idle@example.com", alias: "Idle", active: false, on: false, paused: true },
  ] },
  balanceToken: { takes: false, set: false }, proxy: "",
};
// its vendor refused the provider whole: no key's row carries that rest
const blocked = {
  id: "blocked", name: "Blocked", icon: "generic", chat: "", responses: "", anthropic: "", catalog: "",
  models: [], agents: [], fallback: [], headers: {}, key: { set: true, masked: "sk-…b" },
  keyList: [{ id: "b1", name: "Only", masked: "sk-…b", on: true }],
  rest: { why: "proxy", status: 0, until: soon(30), key: "blocked*" },
  balanceToken: { takes: false, set: false }, proxy: "",
};
const PROVIDERS = [relay, acct, blocked];
const now = Date.now();
const ROWS = [
  { t: new Date(now - 60e3).toISOString(), provider: "relay", providerName: "Relay", providerKeyName: "Main", model: "gpt-6", status: 429, err_type: "rate_limit", err: "slow down" },
  { t: new Date(now - 120e3).toISOString(), provider: "relay", providerName: "Relay", providerKeyName: "Main", model: "gpt-6", status: 429, err_type: "rate_limit", err: "slow down" },
  { t: new Date(now - 180e3).toISOString(), provider: "acct", providerName: "Account", providerAccount: "me@example.com", model: "gpt-6", status: 401, err_type: "auth_error", err: "bad key" },
];

// picture is the providers list after the rests lifted so far: what the page
// reads again once a probe answered
function picture(state) {
  return PROVIDERS.map((p) => {
    const q = JSON.parse(JSON.stringify(p));
    if (q.rest && state.lifted.has(q.rest.key)) delete q.rest;
    for (const k of q.keyList) if (k.rest && state.lifted.has(k.rest.key)) delete k.rest;
    for (const l of q.account?.logins || []) if (l.rest && state.lifted.has(l.rest.key)) delete l.rest;
    return q;
  });
}

function server(lang, state) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/providers") {
      state.reads++;
      return json({ providers: picture(state), presets: [], excluded: [], gateway: { running: true } });
    }
    if (url.pathname === "/api/usage/requests") return json({ rows: ROWS, total: ROWS.length, offset: 0 });
    if (url.pathname === "/api/doctor/model-catalogs") return json({ catalogs: CATS });
    if (url.pathname === "/api/gateway/unrest") {
      const body = req.postDataJSON();
      state.unrest.push({ ...body, at: Date.now() });
      return json({ lifted: true });
    }
    if (url.pathname === "/api/gateway/probe") {
      const body = req.postDataJSON();
      state.probes.push({ ...body, at: Date.now() });
      state.lifted.add(body.key); // the vendor answered: the rest is no more
      return json({ probed: true, lifted: true });
    }
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
  en: {
    title: "Doctor", sub: "Who is resting, why, and what failed lately", rest: "Resting now", fail: "Failed lately",
    try: "Try now", probe: "Probe and recover", all: "Try all", open: "Open provider", asked: "It is asked again now",
    lifted: "It answered: the rest is lifted", every: "Every rest was tried",
    none: "Nothing is resting now.", nofail: "Nothing failed lately.",
    idle: "Not in use", idleCount: "Signed out 1 · Paused 1 · turned off 1",
    restricted: "account restricted by the vendor", appeal: "Appeal with the vendor, then try it again here",
    signin: "Sign in again, then try it again here", top: "Top up the account, then try it again here",
    proxy: "proxy not reachable", signedout: "Signed out", paused: "Paused", off: "turned off",
    keysub: "Key Main", acctsub: "account me@example.com",
    modelsHead: "Real model lists", copyIds: "Copy model IDs", copySteps: "Copy the parsing steps", dumateCount: "3 models",
  },
  zh: {
    title: "医生", sub: "谁在休息、为什么，以及最近哪些请求失败", rest: "正在休息", fail: "最近的失败",
    try: "Try now", probe: "探活并恢复", all: "全部试一遍", open: "打开供应商", asked: "已立刻再问它一次",
    lifted: "它已应答，暂停已解除", every: "全部试过了",
    none: "现在没有 key 或账号在休息。", nofail: "最近没有失败的请求。",
    idle: "未在使用", idleCount: "已退出登录 1 · 已暂停 1 · 已停用 1",
    restricted: "账号被服务商限制", appeal: "找厂商申诉，解除后再来试",
    signin: "重新登录后再来试", top: "充值后再来试",
    proxy: "代理连不上", signedout: "已退出登录", paused: "已暂停", off: "已停用",
    keysub: "密钥 Main", acctsub: "账号 me@example.com",
    modelsHead: "真实模型清单", copyIds: "复制模型 ID", copySteps: "复制解析步骤", dumateCount: "3 个模型",
  },
};
const KEYS = ["Read again", "Resting now", "Failed lately", "Nothing is resting now.", "Nothing failed lately.",
  "Resting is magpie passing a key or account over until its time. Try now asks it again at once; refused again, it rests again.",
  "providers on", "resting", "failed lately", "not taking requests", "{n} keys and accounts", "{n} requests",
  "It is asked again now", "Open provider", "Read {time}",
  "Probe and recover", "Try all", "It answered: the rest is lifted", "Its rest stands", "Every rest was tried",
  "Not in use", "account restricted by the vendor", "Appeal with the vendor, then try it again here",
  "Verify the account with the vendor, then try it again", "Top up the account, then try it again here",
  "Sign in again, then try it again here", "Replace the key, then try it again here"];

// wrap setInterval before the page's own scripts run, so the 45-second read
// can be fired at once instead of waited for
const ticker = () => {
  window.__ticks = [];
  const set = window.setInterval;
  window.setInterval = function (fn, ms, ...rest) {
    if (ms === 45000) window.__ticks.push(fn);
    return set.call(window, fn, ms, ...rest);
  };
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the Doctor page names the key, what to do, and recovers`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1000, height: 900 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      const state = { unrest: [], probes: [], lifted: new Set(), reads: 0 };
      page.on("pageerror", (e) => errors.push(e.message));
      await page.addInitScript(ticker);
      await page.route("http://magpie.test/**", server(lang, state));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="doctor"]').click();
      await page.locator("#view-doctor").waitFor();
      await page.waitForFunction(() => document.querySelector("#drRestCount")?.textContent.includes("5"));
      // what the page is, and the counts
      assert.equal((await page.locator("#drTitle").textContent()).trim(), w.title);
      assert.equal((await page.locator("#drSub").textContent()).trim(), w.sub);
      const cards = await page.locator("#drCards .dr-card b").allTextContents();
      assert.deepEqual(cards, ["3", "5", "3", "3"], "providers on, resting, failed, not in use");
      assert.equal((await page.locator("#drRestHead").textContent()).trim(), w.rest);
      assert.equal((await page.locator("#drFailHead").textContent()).trim(), w.fail);
      // each resting key or account, its why and when — the provider bird's
      // own rest first, as no key's row carries it
      const rests = page.locator("#drRests .dr-rest");
      assert.equal(await rests.count(), 5);
      const subs = await rests.locator(".sub").allTextContents();
      assert.deepEqual(subs, ["Backup · " + (lang === "zh" ? "密钥" : "Key"), "Dead · " + (lang === "zh" ? "密钥" : "Key"),
        "Locked · " + (lang === "zh" ? "密钥" : "Key"), "Work · " + (lang === "zh" ? "账号" : "account"),
        lang === "zh" ? "供应商" : "Provider"]);
      const why = await rests.locator(".dr-why").allTextContents();
      assert(why[0].includes("429") && why[0].length > 4, why[0]);
      assert(why[1].includes("401"), why[1]);
      assert(why[2].includes("403") && why[2].includes(w.restricted), why[2]);
      assert(why[3].includes("429"), why[3]);
      assert.equal(why[4].trim(), w.proxy);
      // a key refused outright is marked bad; a restricted one rests amber
      // with its word, as the Providers page marks it
      assert.equal(await rests.nth(1).evaluate((e) => e.classList.contains("bad")), true);
      assert.equal(await rests.nth(2).evaluate((e) => e.classList.contains("bad")), false);
      const shade = (i) => rests.nth(i).locator(".dr-why").evaluate((e) => getComputedStyle(e).color);
      const amber = await shade(0), red = await shade(1), restricted = await shade(2);
      assert.equal(restricted, amber, "restricted is amber, as a rate limit is");
      assert.notEqual(red, amber, "a refused sign-in is red");
      // what to do, by why and by
      assert.equal(await rests.nth(0).locator(".dr-advice").count(), 0, "a rate limit lifts by itself");
      assert.equal((await rests.nth(1).locator(".dr-advice").textContent()).trim(), w.signin);
      assert.equal((await rests.nth(2).locator(".dr-advice").textContent()).trim(), w.appeal);
      assert.equal((await rests.nth(3).locator(".dr-advice").textContent()).trim(), w.top);
      // signed out, paused and off, counted and named
      assert.equal((await page.locator("#drIdleHead").textContent()).trim(), w.idle);
      assert.equal((await page.locator("#drIdleCount").textContent()).trim(), w.idleCount);
      const idle = page.locator("#drIdle .dr-idle");
      assert.equal(await idle.count(), 3);
      assert.deepEqual(await idle.locator(".name").allTextContents(), ["Relay", "Account", "Account"]);
      const states = await idle.locator(".dr-why").allTextContents();
      assert.deepEqual(states, [w.off, w.signedout, w.paused]);
      // the failed requests, grouped, naming the key or account that failed
      const fails = page.locator("#drFails .dr-fail");
      assert.equal(await fails.count(), 2);
      assert.equal((await fails.nth(0).locator(".name").textContent()).trim(), "Relay");
      assert.equal((await fails.nth(0).locator(".dr-why").textContent()).trim(), "rate_limit");
      assert.equal((await fails.nth(0).locator(".dr-count").textContent()).trim(), "2×");
      assert((await fails.nth(0).locator(".sub").textContent()).includes(w.keysub), await fails.nth(0).locator(".sub").textContent());
      assert.equal((await fails.nth(1).locator(".name").textContent()).trim(), "Account");
      assert((await fails.nth(1).locator(".sub").textContent()).includes(w.acctsub), await fails.nth(1).locator(".sub").textContent());
      // the real model lists the local apps keep, read-only: the IDs to
      // copy, the provider to open when magpie has it, and the steps for a
      // widget no catalog here knows
      await page.waitForFunction(() => !document.querySelector("#drModelsHeadRow")?.hidden);
      assert.equal((await page.locator("#drModelsHead").textContent()).trim(), w.modelsHead);
      const cats = page.locator("#drModels .dr-model");
      assert.equal(await cats.count(), 2);
      assert.equal((await cats.nth(0).locator(".name").textContent()).trim(), "DuMate");
      const sub0 = await cats.nth(0).locator(".sub").textContent();
      assert(sub0.includes("relay · " + w.dumateCount + " ·") && sub0.includes("opencode.json"), sub0);
      assert.equal(await cats.nth(0).getByRole("button", { name: w.copyIds }).count(), 1);
      assert.equal(await cats.nth(0).getByRole("button", { name: w.open }).count(), 1, "the provider magpie has opens");
      assert.equal((await cats.nth(1).locator(".name").textContent()).trim(), "TeleAgent");
      assert.equal(await cats.nth(1).getByRole("button", { name: w.open }).count(), 0, "no provider, no open");
      assert.equal((await page.locator("#drCopySteps").textContent()).trim(), w.copySteps);
      // Try now ends the rest, as the provider's button does
      await rests.nth(0).locator(".dr-try").click();
      for (let i = 0; i < 60 && !state.unrest.length; i++) await page.waitForTimeout(50);
      assert.deepEqual(state.unrest.map(({ key }) => ({ key })), [{ key: "relay#k2" }]);
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.asked);
      // Probe and recover asks the vendor first, and the row goes once it
      // answered
      await rests.nth(2).locator(".dr-probe").click();
      for (let i = 0; i < 60 && !state.probes.length; i++) await page.waitForTimeout(50);
      assert.deepEqual(state.probes.map(({ key }) => ({ key })), [{ key: "relay#k5" }]);
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.lifted);
      await page.waitForFunction(() => document.querySelector("#drRestCount")?.textContent.includes("4"));
      assert.equal(await page.locator("#drRests .dr-rest .name").filter({ hasText: /^Locked$/ }).count(), 0);
      // Try all ends each rest in turn, a fifth of a second apart
      await page.locator("#drTryAll").click();
      for (let i = 0; i < 120 && state.unrest.length < 5; i++) await page.waitForTimeout(50);
      assert.deepEqual(state.unrest.slice(1).map(({ key }) => key), ["relay#k2", "relay#k3", "codex@work", "blocked*"]);
      const gaps = state.unrest.slice(2).map((x, i) => x.at - state.unrest[i + 1].at);
      assert(gaps.every((ms) => ms >= 150), `try all waits between rests: ${gaps}`);
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.every);
      // the page reads itself again every 45 seconds while it is in sight.
      // Wait on the read's own timestamp, not a fixed number of polls: a
      // loaded runner with the whole suite in WebKit can outlast 60×50ms.
      assert.equal(await page.evaluate(() => window.__ticks.length), 1, "one interval, at 45 seconds");
      const reads0 = state.reads;
      await page.evaluate(() => window.__ticks[0]());
      // the read is waited on by its effect (state.reads), not by the clock
      // string: two reads inside one second draw the same time
      for (let i = 0; i < 100 && state.reads <= reads0; i++) await page.waitForTimeout(50);
      assert(state.reads > reads0, "the 45-second read reads the providers again");
      assert.notEqual((await page.locator("#drRead").textContent()).trim(), "", "quietly: it still says when");
      // Open provider goes to it
      await page.locator("#drFails .dr-fail").first().locator(".dr-open").click();
      await page.waitForFunction(() => !document.querySelector("#view-providers")?.hidden);
      const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every new word has its Chinese");
      assert.deepEqual(errors, []);
    });
  }
}