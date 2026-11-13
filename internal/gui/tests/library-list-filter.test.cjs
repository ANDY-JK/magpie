// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 列表功能都统一为 mcp 和技能那种，数量太多操作费劲. The
// Skills tab's filter comes to Hooks and Subagents once there are more than
// six: typing narrows the rows, a word nothing matches says so, Escape
// clears it, and a filter never reaches the page's own keys. English and
// Chinese, Chromium and WebKit; the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/owner";
const agent = (id, name) => ({ id, name, icon: "", skills: `${HOME}/.${id}/skills`, mcp: `${HOME}/.${id}/mcp.json` });
const hook = (name, description, agents) => ({ name, description, event: "after-tool", matcher: "shell", command: "say " + name, timeout: 30, agents });
const sub = (name, description, agents) => ({ name, description, prompt: "You help.", skills: [], mcp: [], agents });
const base = (hooks, subagents, more = {}) => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [agent("claude", "Claude Code"), agent("codex", "Codex")],
  instructions: { agents: [] }, servers: [], skills: [], hooks, hookAgents: [{ id: "claude" }, { id: "codex" }],
  subagents, subagentAgents: [{ id: "claude" }, { id: "codex" }],
  foundServers: [], projects: [], foundSkills: [], problems: [], ...more,
});

function server(lang, state) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/library") return json(base(state.hooks, state.subagents, state.more));
    if (url.pathname === "/api/library/rtk") return json(state.rtk || {});
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const L = {
  en: {
    hooks: "Filter 8 hooks…", sub: "Filter 8 subagents…", tabSub: "Subagents", noneHook: "No hook matches “zzz”.", noneSub: "No subagent matches “zzz”.",
    agents: "Filter 8 agents…", noneAgent: "No agent matches “zzz”.", tabInstr: "Instructions",
  },
  zh: {
    hooks: "筛选 8 个钩子…", sub: "筛选 8 个子代理…", tabSub: "子代理", noneHook: "没有匹配「zzz」的钩子。", noneSub: "没有匹配「zzz」的子代理。",
    agents: "筛选 8 个 Agent…", noneAgent: "没有匹配「zzz」的 Agent。", tabInstr: "指令",
  },
};
const hooks = ["alpha", "bark", "chime", "ding", "echo", "fmt", "gong", "hush"].map((n, i) => hook(n, "hook " + n, i % 2 ? ["claude"] : ["codex"]));
const subs = ["audit", "build", "critic", "docs", "explore", "fix", "grep", "hunt"].map((n) => sub(n, "subagent " + n, ["claude"]));
// eight agents for the Instructions tab and the RTK tab: their rows, too,
// are filtered once there are more than six
const KEYS = ["Filter {n} agents…", "No agent matches “{q}”."];
const NAMES = [["claude", "Claude Code"], ["codex", "Codex"], ["gemini", "Gemini CLI"], ["qwen", "Qwen Code"], ["pi", "Pi"], ["droid", "Droid"], ["cursor", "Cursor"], ["opencode", "OpenCode"]];
const agents = NAMES.map(([id, name]) => agent(id, name));
const INSTR = NAMES.map(([id, name]) => ({ agent: id, name, icon: "", path: `${HOME}/.${id}/AGENTS.md`, on: true }));
const rtk = () => ({
  path: `${HOME}/.local/bin/rtk`, version: "0.51.0", url: "https://www.rtk-ai.app",
  agents: NAMES.map(([id, name]) => ({ id, name, icon: "", on: false })),
});

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: Hooks and Subagents are filtered as Skills are`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const ctx = await browser.newContext({ viewport: { width: 980, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
      await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "hooks"); } catch {} });
      const page = await ctx.newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", server(lang, { hooks, subagents: subs }));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="library"]').click();
      // Hooks: eight rows, a filter, typing narrows, a miss says so, Escape clears
      await page.locator(".lib-hook").first().waitFor();
      assert.equal(await page.locator(".lib-hook").count(), 8);
      assert.equal(await page.locator(".lib-filter").first().getAttribute("placeholder"), w.hooks);
      await page.locator(".lib-filter").first().fill("bark");
      await page.waitForFunction(() => document.querySelectorAll(".lib-hook").length === 1);
      assert.equal((await page.locator(".lib-hook .name").textContent()).trim(), "bark");
      await page.locator(".lib-filter").first().fill("zzz");
      await page.locator(".lib-none").filter({ hasText: w.noneHook }).waitFor();
      await page.locator(".lib-filter").first().press("Escape");
      await page.waitForFunction(() => document.querySelectorAll(".lib-hook").length === 8);
      // Subagents: the same, on their tab
      await page.locator(".lib-tabs .opt", { hasText: w.tabSub }).click();
      await page.locator(".lib-subagent").first().waitFor();
      assert.equal(await page.locator(".lib-subagent").count(), 8);
      const subFilter = page.locator(".lib-filter").first();
      assert.equal(await subFilter.getAttribute("placeholder"), w.sub);
      await subFilter.fill("critic");
      await page.waitForFunction(() => document.querySelectorAll(".lib-subagent").length === 1);
      await subFilter.fill("zzz");
      await page.locator(".lib-none").filter({ hasText: w.noneSub }).waitFor();
      await subFilter.press("Escape");
      await page.waitForFunction(() => document.querySelectorAll(".lib-subagent").length === 8);
      assert.deepEqual(errors, []);
      await ctx.close();
    });

    // the Instructions tab's agents and the RTK tab's rows are filtered the
    // same way once there are more than six: by what each row says, a miss
    // says so, Escape clears it, and a tab away and back keeps what is typed
    test(`${engine} ${lang}: Instructions and RTK agents are filtered as Skills are`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const ctx = await browser.newContext({ viewport: { width: 980, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
      await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "instructions"); } catch {} });
      const page = await ctx.newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", server(lang, { hooks: [], subagents: [], rtk: rtk(), more: { agents, instructions: { shared: "", sets: [], agents: INSTR } } }));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="library"]').click();
      // Instructions: eight agent rows, a filter, typing narrows, a miss says so
      await page.locator(".lib-row").first().waitFor();
      assert.equal(await page.locator(".lib-row").count(), 8);
      const instrFilter = page.locator(".lib-filter").first();
      assert.equal(await instrFilter.getAttribute("placeholder"), w.agents);
      await instrFilter.fill("codex");
      await page.waitForFunction(() => document.querySelectorAll(".lib-row").length === 1);
      assert.equal((await page.locator(".lib-row .name").textContent()).trim(), "Codex");
      await instrFilter.fill("zzz");
      await page.locator(".lib-none").filter({ hasText: w.noneAgent }).waitFor();
      await instrFilter.press("Escape");
      await page.waitForFunction(() => document.querySelectorAll(".lib-row").length === 8);
      assert.equal(await instrFilter.inputValue(), "", "Escape clears it");
      // RTK: the same, on its tab, and what is typed stays across a tab away
      await page.locator(".lib-tabs .opt", { hasText: "RTK" }).click();
      await page.waitForFunction(() => document.querySelectorAll(".lib-row").length === 8);
      const rtkFilter = page.locator(".lib-filter").first();
      assert.equal(await rtkFilter.getAttribute("placeholder"), w.agents);
      await rtkFilter.fill("droid");
      await page.waitForFunction(() => document.querySelectorAll(".lib-row").length === 1);
      assert.equal((await page.locator(".lib-row .name").textContent()).trim(), "Droid");
      await page.locator(".lib-tabs .opt", { hasText: w.tabInstr }).click();
      await page.locator(".lib-tabs .opt", { hasText: "RTK" }).click();
      await page.waitForFunction(() => document.querySelectorAll(".lib-row").length === 1);
      assert.equal(await page.locator(".lib-filter").first().inputValue(), "droid", "what is typed is kept");
      await page.locator(".lib-filter").first().fill("zzz");
      await page.locator(".lib-none").filter({ hasText: w.noneAgent }).waitFor();
      await page.locator(".lib-filter").first().press("Escape");
      await page.waitForFunction(() => document.querySelectorAll(".lib-row").length === 8);
      const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every new word has its Chinese");
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  }
}
