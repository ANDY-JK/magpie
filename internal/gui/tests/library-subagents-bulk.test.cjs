// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): the Library's lists work alike, MCP servers' and
// skills' way. Subagents had one row's actions only; they now have the
// header's Select and Remove all as hooks and servers do: pick some, give
// them to an agent together (subagents/agents-some), or take them out of the
// library together (subagents/remove-all, asked first). English and Chinese,
// both engines; the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/owner";
const agent = (id, name) => ({ id, name, icon: "", skills: `${HOME}/.${id}/skills`, mcp: `${HOME}/.${id}/mcp.json` });
const sub = (name, description, agents) => ({ name, description, prompt: "You help.", skills: [], mcp: [], agents });
const base = (subagents) => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [agent("claude", "Claude Code"), agent("codex", "Codex")],
  instructions: { agents: [] }, servers: [], skills: [], hooks: [], hookAgents: [],
  subagents, subagentAgents: [{ id: "claude" }, { id: "codex" }],
  foundServers: [], projects: [], foundSkills: [], problems: [],
});

function server(lang, state, posts) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/library") return json(base(state.subagents));
    if (url.pathname.startsWith("/api/library/") && req.method() === "POST") {
      const body = req.postDataJSON(), what = url.pathname.replace("/api/library/", "");
      posts.push({ what, body });
      if (what === "subagents/agents-some") {
        state.subagents = state.subagents.map((s) => (body.names.includes(s.name)
          ? { ...s, agents: body.on ? [...new Set([...s.agents, ...body.agents])] : s.agents.filter((a) => !body.agents.includes(a)) } : s));
      }
      if (what === "subagents/remove-all") state.subagents = state.subagents.filter((s) => !body.names.includes(s.name));
      return json({ ...base(state.subagents), result: { changed: ["claude"], problems: [] } });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const L = {
  en: {
    select: "Pick some subagents and turn them on or off together", bar: "Pick subagents to turn them on or off together", sel: "2 selected",
    on: "2 subagents are on for Codex", all: "Remove all 2 subagents?", removed: "2 subagents removed",
  },
  zh: {
    select: "选择一些子代理，一起打开或关闭", bar: "选择子代理，一起打开或关闭", sel: "已选 2 个",
    on: "2 个子代理已对 Codex 打开", all: "移出全部 2 个子代理？", removed: "已移出 2 个子代理",
  },
};
const KEYS = ["Pick some subagents and turn them on or off together", "Stop picking subagents",
  "Pick subagents to turn them on or off together", "{n} subagents are on for {agents}", "{n} subagents are off for {agents}",
  "Take the subagents picked out of the library", "Take all {n} subagents out of the library", "Remove all {n} subagents?",
  "Remove {n} subagents?", "{n} subagents removed", "The files magpie wrote of them are taken out of every agent.",
  "{agent} can take none of these subagents", "{agent} has all of these subagents — click to take them away",
  "{agent} has {n} of these subagents — click to give it the rest", "Give {agent} all of these subagents",
  "Every agent has all of these subagents — click to take them from every one", "Give all of these subagents to every agent"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: subagents are picked and removed together`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const ctx = await browser.newContext({ viewport: { width: 980, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
      await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "subagents"); } catch {} });
      const page = await ctx.newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", server(lang, { subagents: [sub("reviewer", "Reviews a change.", ["claude"]), sub("critic", "Critiques a plan.", ["codex"])] }, posts));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="library"]').click();
      const rows = page.locator(".lib-subagent");
      await rows.first().waitFor();
      assert.equal(await rows.count(), 2);
      const head = page.locator(".lib-subagenthead");
      assert.equal(await head.count(), 1, "the header as hooks have");
      // Select: the bar's chips turn the picked ones on for an agent together
      await head.locator(".lib-select").click();
      const bar = page.locator(".lib-pickbar");
      await bar.filter({ hasText: w.bar }).waitFor();
      for (const box of await rows.locator(".lib-pickbox").all()) await box.click();
      await bar.filter({ hasText: w.sel }).waitFor();
      await bar.locator('.lib-ag[data-agent="codex"]').click();
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.on);
      assert.deepEqual(posts.shift(), { what: "subagents/agents-some", body: { names: ["reviewer", "critic"], agents: ["codex"], on: true } });
      await page.locator(".lib-pickdone").click();
      assert.equal(await page.locator(".lib-pickbox").count(), 0);
      // Remove all asks first, then takes them out together
      await head.locator(".lib-everyrm").click();
      const sheet = page.locator("#modal .lib-editor");
      await sheet.waitFor();
      assert.equal((await sheet.locator(".ehead b").textContent()).trim(), w.all);
      await sheet.locator(".bar button", { hasText: lang === "zh" ? "取消" : "Cancel" }).click();
      await sheet.waitFor({ state: "detached" });
      assert.deepEqual(posts, [], "Cancel posts nothing");
      await head.locator(".lib-everyrm").click();
      await sheet.waitFor();
      await sheet.locator(".bar button.danger-fill").click();
      await sheet.waitFor({ state: "detached" });
      assert.deepEqual(posts.shift(), { what: "subagents/remove-all", body: { names: ["reviewer", "critic"] } });
      await page.waitForFunction(() => document.querySelectorAll(".lib-subagent").length === 0);
      const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every new word has its Chinese");
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  }
}
