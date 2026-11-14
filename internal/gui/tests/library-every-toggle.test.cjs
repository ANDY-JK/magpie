// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-11): the Library's lists work alike, MCP servers' and
// skills' way. Hooks and subagents had one row's chips only; they now have
// the header's Turn all on and Turn all off as skills and servers do: every
// hook (hooks/agents-all) or subagent (subagents/agents-all) given to every
// agent shown at once, and Turn all off asked first. An agent that can't
// take them keeps its greyed chip, which says why. English and Chinese, both
// engines; the API is faked.
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
const base = (hooks, subagents) => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [agent("claude", "Claude Code"), agent("codex", "Codex")],
  instructions: { agents: [] }, servers: [], skills: [],
  // Codex can't be given a hook or a subagent: its chip is greyed and says why
  hooks, hookAgents: [{ id: "claude" }, { id: "codex", no: true }],
  subagents, subagentAgents: [{ id: "claude" }, { id: "codex", no: true }],
  foundServers: [], projects: [], foundSkills: [], problems: [],
});

function server(lang, state, posts) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/library") return json(base(state.hooks, state.subagents));
    if (url.pathname.startsWith("/api/library/") && req.method() === "POST") {
      const body = req.postDataJSON(), what = url.pathname.replace("/api/library/", "");
      posts.push({ what, body });
      const give = (rows) => rows.map((x) => (body.agents.includes("claude") && body.agents.includes("codex")
        ? { ...x, agents: body.on ? [...new Set([...x.agents, ...body.agents])] : [] } : x));
      if (what === "hooks/agents-all") state.hooks = give(state.hooks);
      if (what === "subagents/agents-all") state.subagents = give(state.subagents);
      return json({ ...base(state.hooks, state.subagents), result: { changed: ["claude"], problems: [] } });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const L = {
  en: {
    cancel: "Cancel", on: "2 hooks are on for all 2 agents", offHead: "Turn off all 2 hooks?",
    onSub: "2 subagents are on for all 2 agents", tabSub: "Subagents",
    noHook: "Codex has no hooks magpie can write", noSub: "Codex reads no subagents magpie can write",
  },
  zh: {
    cancel: "取消", on: "已为全部 2 个 Agent 启用 2 个 Hook", offHead: "关闭全部 2 个 Hook？",
    onSub: "已为全部 2 个 Agent 启用 2 个子代理", tabSub: "子代理",
    noHook: "magpie 还不能给 Codex 写入 Hook", noSub: "Codex 没有 magpie 能写入的子代理",
  },
};
const KEYS = ["Turn all on", "Turn all off", "Cancel",
  "Give every hook to all {n} agents that can run hooks", "Take every hook from all {n} agents",
  "Turn off all {n} hooks?", "Every hook is taken out of {agents}. They stay in the library, to turn on again.",
  "{n} hooks are on for all {m} agents", "{n} hooks are off for all {m} agents",
  "Give every subagent to all {n} agents that can take them", "Take every subagent from all {n} agents",
  "Turn off all {n} subagents?", "Every subagent is taken out of {agents}. They stay in the library, to turn on again.",
  "{n} subagents are on for all {m} agents", "{n} subagents are off for all {m} agents",
  "{agent} has no hooks magpie can write", "{agent} reads no subagents magpie can write"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: hooks and subagents are turned on and off for every agent at once`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const ctx = await browser.newContext({ viewport: { width: 980, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
      await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "hooks"); } catch {} });
      const page = await ctx.newPage();
      t.after(() => browser.close());
      // room on both sides: the whole batch shares the machine, and a
  // 5-second wait went red under it where 10 is quiet
  page.setDefaultTimeout(10000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const state = { hooks: [hook("fmt", "Formats", ["claude"]), hook("ping", "Pings", [])], subagents: [sub("reviewer", "Reviews", ["claude"]), sub("critic", "Critiques", [])] };
      await page.route("http://magpie.test/**", server(lang, state, posts));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="library"]').click();
      const rows = page.locator(".lib-hook");
      await rows.first().waitFor();
      assert.equal(await rows.count(), 2);
      const head = page.locator(".lib-hookhead");
      assert.equal(await head.count(), 1, "the header as skills have");
      // Codex can't be given a hook: its chip is greyed, and a click says why
      const codex = page.locator('.lib-hook .lib-ag[data-agent="codex"]').first();
      assert.equal(await codex.getAttribute("aria-disabled"), "true");
      assert.ok((await codex.getAttribute("class")).includes("blocked"), "the greyed chip");
      await codex.click({ force: true }); // aria-disabled: it says why, it doesn't switch
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.noHook);
      assert.deepEqual(posts, [], "a greyed chip posts nothing");
      // Turn all off asks first, and Cancel posts nothing
      await head.locator(".lib-everyoff").click();
      const sheet = page.locator("#modal .lib-editor");
      await sheet.waitFor();
      assert.equal((await sheet.locator(".ehead b").textContent()).trim(), w.offHead);
      await sheet.locator(".bar button", { hasText: w.cancel }).click();
      await sheet.waitFor({ state: "detached" });
      assert.deepEqual(posts, [], "Cancel posts nothing");
      await head.locator(".lib-everyoff").click();
      await sheet.waitFor();
      await sheet.locator(".bar button.danger-fill").click();
      await sheet.waitFor({ state: "detached" });
      assert.deepEqual(posts.shift(), { what: "hooks/agents-all", body: { agents: ["claude", "codex"], on: false } });
      // Turn all on gives every hook to every agent shown at once
      await head.locator(".lib-everyon").click();
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.on);
      assert.deepEqual(posts.shift(), { what: "hooks/agents-all", body: { agents: ["claude", "codex"], on: true } });
      // the same two buttons on the Subagents tab
      await page.locator(".lib-tabs .opt", { hasText: w.tabSub }).click();
      await page.locator(".lib-subagent").first().waitFor();
      const subCodex = page.locator('.lib-subagent .lib-ag[data-agent="codex"]').first();
      assert.equal(await subCodex.getAttribute("aria-disabled"), "true");
      await subCodex.click({ force: true });
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.noSub);
      assert.deepEqual(posts, [], "a greyed chip posts nothing");
      await page.locator(".lib-subagenthead .lib-everyon").click();
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.onSub);
      assert.deepEqual(posts.shift(), { what: "subagents/agents-all", body: { agents: ["claude", "codex"], on: true } });
      const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every new word has its Chinese");
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  }
}