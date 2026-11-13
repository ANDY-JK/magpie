// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 拆二级 tab，避免滚动很久. Usage › Sessions has its own tabs
// above the figures — All (the default, the pane as it was), Distribution (the
// activity chart and the cards that say what the sessions were made of) and
// Ranking (the sessions, tools and skills that came top). The figures and the
// list stay under every pick, a group with nothing in the range has no tab, the
// pick is remembered, and the cards drawn again by a later read do not put the
// page back to All. A press on a tab moves the page not at all. English and
// Chinese, Chromium and WebKit; the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { inView } = require("./reader.cjs");

const assets = path.resolve(__dirname, "../assets");
const iso = (d) => [d.getFullYear(), d.getMonth() + 1, d.getDate()].map((n) => String(n).padStart(2, "0")).join("-");

// thirty days at work, ending today, so every card has something to say
const N = 30;
const DAYS = Array.from({ length: N }, (_, i) => {
  const d = new Date();
  d.setDate(d.getDate() - (N - 1 - i));
  const n = 1 + (i % 4);
  return {
    date: iso(d),
    usage: [
      { agent: "claude", cwd: "/work/alpha", model: "claude-opus-4", input: 30000 * n, output: 4000 * n, cache_read: 6000 * n, cache_write: 0, cost: 1.5 * n, priced: true },
      { agent: "claude", cwd: "/work/beta", model: "claude-sonnet-5", input: 9000 * n, output: 1500 * n, cache_read: 0, cache_write: 0, cost: 0.25 * n, priced: true },
    ],
    active: [{ agent: "claude", cwd: "/work/alpha", seconds: 600 * n, hours: Object.assign(new Array(24).fill(0), { 9: 600 * n, 15: 300 * n }) }],
  };
});
const TOP_TOKENS = DAYS.slice(-3).map((d, i) => ({
  key: "claude:s" + i, agent: "claude", id: "s" + i, cwd: "/work/alpha", title: "a session " + (i + 1),
  last: d.date + "T10:00:00Z", input: 120000, output: 9000, cache_read: 40000, cache_write: 0, cost: 1.2, priced: true, active: 3600,
  models: ["claude-opus-4"],
}));
const SHAPE = (counts) => ({ edges: [1, 6, 16, 31, 61, 121], counts, total: counts.reduce((a, b) => a + b, 0) });
const TOOLS = {
  calls: 44, sessions: 9,
  top: [{ name: "Read", category: "Read", calls: 24, sessions: 9 }, { name: "Bash", category: "Bash", calls: 20, sessions: 7 }],
  categories: [{ name: "Read", calls: 24 }, { name: "Bash", calls: 20 }],
  weeks: [{ start: DAYS[7].date, calls: { Read: 10, Bash: 8 } }, { start: DAYS[N - 1].date, calls: { Read: 14, Bash: 12 } }],
};
const SKILLS = {
  calls: 12, count: 2,
  top: [{ name: "pdf", calls: 8, sessions: 4, last: DAYS[N - 1].date, agents: { claude: 8 }, projects: [{ name: "/work/alpha", calls: 8 }] }],
};
const SESSIONS = [0, 1, 2].map((i) => ({
  agent: "claude", name: "Claude Code", icon: "claudecode-color", id: "s" + i, cwd: "/work/alpha",
  title: "a session " + (i + 1), last: DAYS[N - 1 - i].date + "T10:00:00Z",
  models: [{ model: "claude-opus-4", input: 120000, output: 9000, cache_read: 40000, cache_write: 0, cost: 1.2, priced: true }],
  input: 120000, output: 9000, cache_read: 40000, cache_write: 0, cost: 1.2, unpriced: 0,
}));

// the range asked for, as the server would answer it: 0 is every day
const daysIn = (q) => {
  const n = Number(q.get("days")) || 0;
  return n > 0 ? DAYS.slice(-n) : DAYS;
};

// serve draws the Sessions pane. empty: "rank" leaves the ranking cards with
// nothing in the range, "all" leaves no session at all.
function serve(lang, empty = "") {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/sessions/progress") return json({ indexing: false });
    if (url.pathname === "/api/sessions") return json({ sessions: empty === "all" ? [] : SESSIONS, dirs: ["/test/sessions"] });
    if (url.pathname === "/api/sessions/stats") {
      if (empty === "all") return json({ from: "", to: "", days: [], agents: {} });
      const days = daysIn(url.searchParams);
      return json({ from: days[0].date, to: days[days.length - 1].date, days, agents: { claude: "Claude Code" } });
    }
    if (url.pathname === "/api/sessions/overview") {
      if (empty === "all") return json({ count: 0, median: 0, p90: 0, days: [], messages: [], output: [], top: {}, shape: {}, tools: {}, skills: {} });
      const days = daysIn(url.searchParams);
      const n = days.length;
      return json({
        count: 11, median: 120000, p90: 880000,
        days: days.map(() => 2), messages: days.map(() => 30), output: days.map(() => 4000),
        top: empty === "rank" ? {} : { tokens: TOP_TOKENS, cost: TOP_TOKENS, active: TOP_TOKENS },
        shape: empty === "rank" ? {} : { messages: SHAPE([5, 3, 2, 1, 0, 0]), minutes: SHAPE([1, 6, 3, 1, 0, 0]), autonomy: SHAPE([2, 5, 3, 1, 0, 0]) },
        tools: empty === "rank" ? { calls: 0, sessions: 0, top: [], categories: [], weeks: [] } : TOOLS,
        skills: empty === "rank" ? { calls: 0, count: 0, top: [] } : SKILLS,
        // the chart is drawn day by day for a range this long
        range: n,
      });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    const body = await fs.readFile(file).catch(() => null);
    return body ? route.fulfill({ body, contentType }) : route.fulfill({ status: 404, body: "" });
  };
}

const L = {
  en: { tabs: ["All", "Distribution", "Ranking"], seven: "7 days", trend: "Requests over time", heat: "Usage heatmap" },
  zh: { tabs: ["全部", "分布", "排行"], seven: "7 天", trend: "请求趋势", heat: "用量热力图" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: Sessions' own tabs show a group alone`, async (t) => {
      assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const context = await browser.newContext({ viewport: { width: 1180, height: 640 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
      const page = await context.newPage();
      page.setDefaultTimeout(15000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang));
      await page.addInitScript(() => { localStorage.setItem("magpie.usageTab", "sessions"); localStorage.setItem("magpie.sessRange", "all"); });
      await page.goto("http://magpie.test/?view=usage");

      const tabs = page.locator("#sessTabs");
      await tabs.waitFor({ state: "visible" });
      await page.locator("#sessChart").waitFor({ state: "visible" });
      const shown = (sel) => page.locator(sel).evaluate((e) => !!e.getClientRects().length);
      const onTab = () => page.locator("#sessTabs .opt.on").innerText().then((s) => s.trim());

      // the three tabs, All first and the pick
      assert.deepEqual((await tabs.locator("button:not([hidden])").allTextContents()).map((x) => x.trim()), w.tabs, "All, Distribution and Ranking");
      assert.equal(await onTab(), w.tabs[0]);
      // All: everything, as the pane was
      for (const sel of ["#sessChart", "#sessProjects", "#sessModels", "#sessHours", "#sessShape", "#sessTop", "#sessTools", "#sessSkills"]) {
        assert.equal(await shown(sel), true, `All shows ${sel}`);
      }
      // the figures and the list are the pane's own, under every pick
      const resident = async () => {
        assert(await page.locator("#sessStats .kpi").count() >= 6, "the figures stay");
        assert.equal(await shown("#sessStats"), true, "the figures stay in sight");
        assert.equal(await shown("#sessListHead"), true, "the list's head stays");
        assert(await page.locator("#sessList .sess-item").count() > 0, "the list stays");
      };
      await resident();

      // Distribution: the chart and what the sessions were made of
      await tabs.locator('button[data-stab="dist"]').click();
      await page.waitForTimeout(200);
      assert.equal(await onTab(), w.tabs[1]);
      for (const sel of ["#sessChart", "#sessProjects", "#sessModels", "#sessHours", "#sessShape"]) {
        assert.equal(await shown(sel), true, `Distribution shows ${sel}`);
      }
      for (const sel of ["#sessTop", "#sessTools", "#sessSkills"]) {
        assert.equal(await shown(sel), false, `Distribution puts ${sel} away`);
      }
      await resident();

      // Ranking: the sessions, tools and skills that came top
      await tabs.locator('button[data-stab="rank"]').click();
      await page.waitForTimeout(200);
      assert.equal(await onTab(), w.tabs[2]);
      for (const sel of ["#sessTop", "#sessTools", "#sessSkills"]) {
        assert.equal(await shown(sel), true, `Ranking shows ${sel}`);
      }
      for (const sel of ["#sessChart", "#sessProjects", "#sessModels", "#sessHours", "#sessShape"]) {
        assert.equal(await shown(sel), false, `Ranking puts ${sel} away`);
      }
      await resident();

      // remembered across a reload, and worked out again as the pane loads
      await page.reload();
      await page.locator("#sessChart").waitFor({ state: "attached" });
      await tabs.waitFor({ state: "visible" });
      await page.waitForTimeout(400);
      assert.equal(await onTab(), w.tabs[2], "Ranking is still the pick");
      assert.equal(await shown("#sessTools"), true, "Ranking's cards are back");
      assert.equal(await shown("#sessChart"), false, "Distribution's are out of the way");
      await resident();

      // drawn again by a later read: the pick holds (the cards set their own
      // hidden as they are drawn, so a redraw that re-applied nothing would
      // put the page back to All — this asks for another range)
      await tabs.locator('button[data-stab="dist"]').click();
      await page.waitForTimeout(200);
      const asked = [];
      const seen = (req) => { const u = new URL(req.url()); if (u.pathname === "/api/sessions/stats") asked.push(u.searchParams.get("days")); };
      page.on("request", seen);
      await page.locator("#sessRange .opt", { hasText: w.seven }).click();
      for (let i = 0; i < 60 && !asked.includes("7"); i++) await page.waitForTimeout(50);
      assert(asked.includes("7"), "the range's read was made: " + JSON.stringify(asked));
      page.off("request", seen);
      await page.waitForTimeout(300);
      assert.equal(await onTab(), w.tabs[1], "the redraw leaves the pick where it was, not on All");
      assert.equal(await shown("#sessChart"), true, "Distribution's cards are drawn and shown");
      assert.equal(await shown("#sessTools"), false, "Ranking's stay put away after the redraw");
      await resident();

      // a press on a tab moves nothing: the page stays where the reader left it
      const view = page.locator("#view-usage");
      const rank = tabs.locator('button[data-stab="rank"]');
      await page.mouse.move(600, 400);
      await page.mouse.wheel(0, 300);
      await page.waitForTimeout(300);
      await inView(page, rank);
      const left = await view.evaluate((v) => v.scrollTop);
      assert(left > 0, "the page is scrolled");
      await rank.click();
      await page.waitForTimeout(300);
      assert.equal(await view.evaluate((v) => v.scrollTop), left, "the press left the page where it was");
      assert.deepEqual(errors, []);
      await context.close();
    });

    test(`${engine} ${lang}: a group with nothing in the range has no tab`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 1180, height: 640 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(15000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, "rank"));
      await page.addInitScript(() => { localStorage.setItem("magpie.usageTab", "sessions"); localStorage.setItem("magpie.sessRange", "all"); });
      await page.goto("http://magpie.test/?view=usage");
      const tabs = page.locator("#sessTabs");
      await tabs.waitFor({ state: "visible" });
      await page.locator("#sessChart").waitFor({ state: "visible" });
      // nothing came top: no Ranking tab, and the pane is the two that have
      assert.deepEqual((await tabs.locator("button:not([hidden])").allTextContents()).map((x) => x.trim()), [w.tabs[0], w.tabs[1]], "Ranking has no tab");
      assert.equal(await tabs.locator('button[data-stab="rank"]').isHidden(), true);
      // a remembered Ranking comes back to All, its group having nothing
      await page.evaluate(() => localStorage.setItem("magpie.sessTab", "rank"));
      await page.reload();
      await tabs.waitFor({ state: "visible" });
      await page.locator("#sessChart").waitFor({ state: "visible" });
      await page.waitForTimeout(300);
      assert.equal((await page.locator("#sessTabs .opt.on").innerText()).trim(), w.tabs[0], "All, as the only pick there is");
      assert.equal(await page.locator("#sessChart").evaluate((e) => !!e.getClientRects().length), true);
      assert.deepEqual(errors, []);
      await page.close();
    });

    test(`${engine} ${lang}: no sessions at all leaves no tabs`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 1180, height: 640 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(15000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, "all"));
      await page.addInitScript(() => { localStorage.setItem("magpie.usageTab", "sessions"); localStorage.setItem("magpie.sessRange", "all"); });
      await page.goto("http://magpie.test/?view=usage");
      await page.waitForFunction(() => !document.querySelector("#view-usage").classList.contains("loading") && document.querySelector("#sessStats").childElementCount > 0);
      await page.waitForTimeout(300);
      // nothing to group: the strip is gone, and the figures say why
      assert.equal(await page.locator("#sessTabs").isHidden(), true, "no tab to pick");
      assert(await page.locator("#sessStats .none").count() > 0, "the pane says there are no sessions");
      assert.deepEqual(errors, []);
      await page.close();
    });
  }
}