// Run with Node's test runner and Playwright on the module path; see README.md.
// Load from files reads the export folder back: a dry run says what would
// change, the page asks first, and Load writes the edits into the library.
// English and Chinese, Chromium and WebKit; the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/owner";
const agent = { id: "claude", name: "Claude Code", icon: "", skills: `${HOME}/.claude/skills`, mcp: `${HOME}/.claude/mcp.json` };
const server = (name) => ({ name, description: "Issues", transport: "stdio", command: "npx", agents: ["claude"] });
const lib = (servers) => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME, agents: [agent],
  instructions: { agents: [], sets: [] }, skills: [], hookAgents: [{ id: "claude" }], subagentAgents: [{ id: "claude" }],
  servers, hooks: [], subagents: [], foundServers: [], projects: [], foundSkills: [], problems: [],
});

function serve(lang, state, posts) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/library") return json(lib(state.servers));
    if (url.pathname === "/api/library/import" && req.method() === "POST") {
      const body = req.postDataJSON();
      posts.push(body);
      if (body.apply) { state.servers = [...state.servers, server("two")]; return json({ dir: `${HOME}/.magpie/library/export`, adds: 1, updates: 0, same: 0, applied: true }); }
      return json({ dir: `${HOME}/.magpie/library/export`, adds: 1, updates: 0, same: 0 });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}
const L = {
  en: { btn: "Load from files", ask: "Load the files into the library?", load: "Load", said: "1 added, 0 updated" },
  zh: { btn: "从文件载入", ask: "把文件载入资源库？", load: "载入", said: "新增 1 项、更新 0 项" },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the Library loads the files back`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const ctx = await browser.newContext({ viewport: { width: 980, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" });
      await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "mcp"); } catch {} });
      const page = await ctx.newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", serve(lang, { servers: [server("one")] }, posts));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="library"]').click();
      const imp = page.locator(".lib-import");
      await imp.waitFor();
      assert.equal((await imp.textContent()).trim(), w.btn);
      await imp.click();
      const dialog = page.locator("dialog.action-confirm");
      await dialog.waitFor();
      assert.equal((await dialog.locator("h2").textContent()).trim(), w.ask);
      await dialog.getByRole("button", { name: w.load, exact: true }).click();
      let said = "";
      for (let i = 0; i < 50 && said !== w.said; i++) { await page.waitForTimeout(100); said = await page.locator("#status").textContent(); }
      assert.equal(said, w.said, "the status says what was loaded");
      assert.deepEqual(posts, [{ apply: false }, { apply: true }], "asked, then loaded");
      await page.locator(".row.lib-row", { hasText: "two" }).waitFor();
      const missing = ["Load from files", "Load the files into the library?", "Load", "{adds} added, {updates} updated"].filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every word has its Chinese");
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  }
}
