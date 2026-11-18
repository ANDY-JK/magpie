// Run with Node's test runner and Playwright on the module path; see README.md.
// The Metrics page (this change): what the gateway and this process hold and
// spend right now. Three reads — /api/busy (what a restart would cut short,
// and each request in flight with its agent, model, provider, phase and
// time), /api/process (memory, CPU, goroutines, with the sampler's last four
// hours as two small canvas charts) and /api/usage/hourly (calls and
// failures by hour, with their averages) — and links to the pages that
// already show the rest. When the ledger couldn't write usage records
// (usageDrops), the process panel says how many, so a Usage total short of
// the calls made is explained; none says nothing. A read that fails keeps the page: the section says
// so where its numbers would be, and what was read before stays drawn.
// Nothing here scrolls by itself. English and Chinese, Chromium and WebKit;
// no backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");

const BUSY = {
  owner: true,
  busy: { requests: 2, tools: 1 },
  active: [
    { id: 1, agent: "codex", model: "gpt-6", provider: "Relay", ms: 830 },
    { id: 2, agent: "claude", model: "opus-6", provider: "Account", ms: 12400, phase: "slot" },
  ],
};
const now = Date.now();
const HIST = [0, 15, 30, 45].map((s, i) => ({
  t: new Date(now - (45 - s) * 1000).toISOString(),
  heapAlloc: 320e6 + i * 4e6,
  heapSys: 512e6,
  heapReleased: 120e6,
  goroutines: 40 + i,
  cpuTotal: 100 + i * 1.5,
}));
const PROC = {
  pid: 4242, program: "magpie.exe", version: "0.1.400", window: true, uptime: "2h3m4s",
  heapAlloc: 328e6, heapInuse: 300e6, heapIdle: 100e6, heapReleased: 120e6, heapSys: 512e6,
  stackSys: 8e6, sys: 560e6, numGC: 91, goroutines: 43, cpuKnown: true, cpuTotal: 104.5,
  numCPU: 8, gomaxprocs: 8, history: HIST,
};
const PROC_DROPS = { ...PROC, usageDrops: 7 };
const CALLS = [3, 0, 5, 9, 4, 0, 12, 7];
const HOURS = {
  since: new Date(now - 24 * 3600e3).toISOString(), hours: 24,
  points: Array.from({ length: 24 }, (_, i) => ({
    t: new Date(now - (23 - i) * 3600e3).toISOString(),
    calls: CALLS[i % 8], errors: i % 5 === 0 ? 1 : 0, tokens: 1000 * i, cost: 0.01 * i,
  })),
};

function server(lang, state) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/busy") return state.busyFail ? route.fulfill({ status: 500, body: "" }) : json(BUSY);
    if (url.pathname === "/api/process") return json(state.drops ? PROC_DROPS : PROC);
    if (url.pathname === "/api/usage/hourly") return state.hourFail ? route.fulfill({ status: 500, body: "" }) : json(HOURS);
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true } });
    if (url.pathname === "/api/usage/requests") return json({ rows: [], total: 0, offset: 0 });
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
    title: "Metrics", sub: "Requests in flight, this process's memory and CPU, and calls by hour",
    reload: "Read now", live: "Running now", requests: "Requests", tools: "Turns waiting on tools",
    slot: "queued for a key or account", proc: "The process itself",
    mem: "Memory, the last four hours", cpu: "CPU, the last four hours",
    memk: "live memory", sysk: "taken from the system", backk: "returned so far", gok: "Go routines",
    cpuk: "CPU share", upk: "running since", dropk: "usage records not written",
    hour: "Calls by hour", avg: "An average of 5 calls an hour, 0.2 failed",
    callsH: "Calls an hour", failedH: "Failed an hour",
    more: "Shown elsewhere", usageSub: "Which provider answered, and what it cost",
    routingSub: "Which route served what, and its totals", doctorSub: "Who rests, and what failed lately",
    open: "Open",
  },
  zh: {
    title: "指标", sub: "进行中的请求、本进程的内存与 CPU，以及按小时的调用",
    reload: "立即读取", live: "正在运行", requests: "请求", tools: "等待工具结果的回合",
    slot: "在 key 或账号的队列中", proc: "本进程",
    mem: "内存，最近四小时", cpu: "CPU，最近四小时",
    memk: "在用内存", sysk: "向系统申请的", backk: "已归还的", gok: "Go 例程",
    cpuk: "CPU 占用", upk: "已运行", dropk: "未写入的用量记录",
    hour: "按小时的调用", avg: "平均每小时 5 次调用，0.2 次失败",
    callsH: "每小时调用", failedH: "每小时失败",
    more: "别处已有", usageSub: "哪个供应商应答、花了多少",
    routingSub: "哪条路由服务了什么，以及总计", doctorSub: "谁在休息、最近什么失败",
    open: "打开",
  },
};
const KEYS = ["Read now", "Read at {time}", "Running now", "Turns waiting on tools",
  "Waiting ones say where they wait: a key's queue, or room in the minute.",
  "The gateway is served by another magpie on this computer.",
  "queued for a key or account", "waiting for the minute", "asking a vendor", "masking its words",
  "The process itself", "live memory", "taken from the system", "returned so far", "Go routines",
  "CPU share", "running since", "usage records not written", "Memory, the last four hours", "CPU, the last four hours",
  "Idle past half a minute, the gateway hands its memory back.", "Calls by hour", "Calls an hour",
  "Failed an hour", "An average of {n} calls an hour, {m} failed",
  "Waiting requests aren't counted by hour yet.", "Shown elsewhere",
  "Which provider answered, and what it cost", "Which route served what, and its totals",
  "Who rests, and what failed lately"];

// wrap setInterval before the page's own scripts run, so the 15-second read
// can be fired at once instead of waited for
const ticker = () => {
  window.__ticks = [];
  const set = window.setInterval;
  window.setInterval = function (fn, ms, ...rest) {
    if (ms === 15000) window.__ticks.push(fn);
    return set.call(window, fn, ms, ...rest);
  };
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the Metrics page draws its three reads and links onward`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1000, height: 900 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      const state = { busyFail: false, hourFail: false, drops: true };
      page.on("pageerror", (e) => errors.push(e.message));
      await page.addInitScript(ticker);
      await page.route("http://magpie.test/**", server(lang, state));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="metrics"]').click();
      await page.locator("#view-metrics").waitFor();
      await page.waitForFunction(() => document.querySelectorAll("#mtLive .kpi").length === 2);
      // what the page is
      assert.equal((await page.locator("#mtTitle").textContent()).trim(), w.title);
      assert.equal((await page.locator("#mtSub").textContent()).trim(), w.sub);
      assert.equal((await page.locator("#mtReload").textContent()).trim(), w.reload);
      assert.equal((await page.locator("#mtLiveHead").textContent()).trim(), w.live);
      assert((await page.locator("#mtRead").textContent()).trim().length > 0, "the read says when");
      // what the gateway holds: the two counts, and each request in flight
      const cells = await page.locator("#mtLive .kpi").all();
      assert.deepEqual(await Promise.all(cells.map((c) => c.locator("b").textContent())), ["2", "1"]);
      assert.deepEqual(await Promise.all(cells.map((c) => c.locator("span").textContent())), [w.requests, w.tools]);
      const rows = page.locator("#mtActive .row");
      assert.equal(await rows.count(), 2);
      const sub1 = (await rows.nth(0).locator(".sub").textContent()).trim();
      assert.equal(sub1, "gpt-6 · Relay · 830 ms", sub1);
      const sub2 = (await rows.nth(1).locator(".sub").textContent()).trim();
      assert.equal(sub2, `opus-6 · Account · ${w.slot} · 12.4 s`, sub2);
      assert.equal((await page.locator("#mtPhase").textContent()).trim(),
        lang === "zh" ? "排队的请求会说明在等什么：等某个 key 的队列，或等这一分钟的余量。" : "Waiting ones say where they wait: a key's queue, or room in the minute.");
      // the process's own numbers
      assert.equal((await page.locator("#mtProcHead").textContent()).trim(), w.proc);
      assert((await page.locator("#mtProcMeta").textContent()).includes("pid 4242"));
      const pk = await page.locator("#mtProc .kpi").all();
      assert.deepEqual(await Promise.all(pk.map((c) => c.locator("b").textContent())),
        ["328 MB", "560 MB", "120 MB", "43", "1%", "2h3m4s", "7"]);
      assert.deepEqual(await Promise.all(pk.map((c) => c.locator("span").textContent())),
        [w.memk, w.sysk, w.backk, w.gok, w.cpuk, w.upk, w.dropk]);
      assert.equal((await page.locator("#mtMemHead").textContent()).trim(), w.mem);
      assert.equal((await page.locator("#mtCpuHead").textContent()).trim(), w.cpu);
      // the three charts are painted, and the idle note is there
      assert.deepEqual(await page.evaluate(() => ["#mtMem", "#mtCpu", "#mtHour"].map((id) => {
        const cv = document.querySelector(id);
        return cv.width > 100 && cv.getContext("2d").getImageData(0, 0, cv.width, cv.height).data.some((v) => v !== 0);
      })), [true, true, true]);
      assert.equal((await page.locator("#mtIdle").textContent()).trim(),
        lang === "zh" ? "空闲超过半分钟，网关就把内存交还给系统。" : "Idle past half a minute, the gateway hands its memory back.");
      // calls by hour: the averages, the two-series legend
      assert.equal((await page.locator("#mtHourAvg").textContent()).trim(), w.avg);
      const legend = (await page.locator("#mtHourLegend").textContent()).trim();
      assert(legend.includes(w.callsH) && legend.includes(w.failedH), legend);
      assert.equal((await page.locator("#mtHourNote").textContent()).trim(),
        lang === "zh" ? "排队的请求还没有按小时统计。" : "Waiting requests aren't counted by hour yet.");
      // links to the pages that already show the rest; Doctor's opens it
      assert.equal((await page.locator("#mtMoreHead").textContent()).trim(), w.more);
      const more = page.locator("#mtMore .row");
      assert.equal(await more.count(), 3);
      assert.deepEqual(await more.locator(".name").allTextContents(), [lang === "zh" ? "用量" : "Usage", lang === "zh" ? "路由" : "Routing", lang === "zh" ? "医生" : "Doctor"]);
      assert.deepEqual(await more.locator(".sub").allTextContents(), [w.usageSub, w.routingSub, w.doctorSub]);
      assert.deepEqual(await more.locator("button").allTextContents(), [w.open, w.open, w.open]);
      await more.nth(2).locator("button").click();
      await page.waitForFunction(() => !document.querySelector("#view-doctor")?.hidden);
      // the page's own interval, at 15 seconds; its read draws again. The
      // shim catches every 15-second beat the window runs (app.js's drift
      // heartbeat and redact's have the same step), so fire them all: each
      // guards its own page and only the Metrics page's beat reads here.
      assert((await page.evaluate(() => window.__ticks.length)) >= 1, "the page keeps a 15-second beat");
      await page.evaluate(() => window.__ticks.forEach((f) => f()));
      await page.waitForFunction(() => document.querySelectorAll("#mtActive .row").length === 2);
      // nothing in the page scrolls by itself
      const scrollers = await page.evaluate(() => [...document.querySelectorAll("#view-metrics *, #view-metrics")]
        .filter((n) => {
          const s = getComputedStyle(n);
          return (s.overflowY === "auto" || s.overflowY === "scroll") && n.scrollHeight > n.clientHeight + 1;
        }).length);
      assert.equal(scrollers, 0, "no box of the page scrolls on its own");
      assert.deepEqual(errors, []);
    });

    test(`${engine} ${lang}: a read that fails keeps the page, and says so`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1000, height: 900 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      const state = { busyFail: true, hourFail: true };
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", server(lang, state));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="metrics"]').click();
      await page.locator("#view-metrics").waitFor();
      // busy's placeholder is drawn where its numbers would be; the page as
      // a whole is there, and the reads that worked still show
      // no drops (the fixture without usageDrops): the panel says nothing
      // of them — the seven KPIs above need state.drops
      await page.waitForFunction(() => document.querySelectorAll("#mtProc .kpi").length === 6);
      await page.waitForFunction(() => (document.querySelector("#mtLive .none")?.textContent || "").length > 0);
      assert.equal((await page.locator("#mtTitle").textContent()).trim(), w.title);
      assert.equal(await page.locator("#mtLive .none").count(), 1);
      assert.equal(await page.locator("#mtActive .row").count(), 0);
      assert.equal((await page.locator("#mtHourAvg").textContent()).trim().length > 0, true, "the hour read says why it is empty");
      assert.equal(await page.locator("#mtHourLegend").count(), 1);
      assert.equal((await page.locator("#mtHourLegend").textContent()).trim(), "", "no legend without points");
      // recovery: the reads answering, the page fills in without a reload
      state.busyFail = false;
      state.hourFail = false;
      await page.locator("#mtReload").click();
      await page.waitForFunction(() => document.querySelectorAll("#mtLive .kpi").length === 2);
      await page.waitForFunction((word) => (document.querySelector("#mtHourAvg")?.textContent || "").includes(word), lang === "zh" ? "平均" : "An average of");
      assert.equal(await page.locator("#mtLive .none").count(), 0);
      assert.equal(await page.locator("#mtActive .row").count(), 2);
      assert.deepEqual(errors, []);
    });
  }
}

// every word the page says has its Chinese, from the plugin's own file
test("the Metrics page's words have their Chinese", () => {
  const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
  assert.deepEqual(missing, [], "every new word has its Chinese");
});
