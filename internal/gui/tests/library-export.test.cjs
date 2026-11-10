// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 像技能一样建立对应得目录. The Library's header has Save
// as files: it writes the MCP servers, hooks and subagents as files under
// the library's export folder (a mirror magpie never reads back), says how
// many, and opens the folder. English and Chinese, Chromium and WebKit; the
// API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/owner";
const agent = { id: "claude", name: "Claude Code", icon: "", skills: `${HOME}/.claude/skills`, mcp: `${HOME}/.claude/mcp.json` };
const lib = {
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME, agents: [agent],
  instructions: { agents: [] }, skills: [], hookAgents: [{ id: "claude" }], subagentAgents: [{ id: "claude" }],
  servers: [{ name: "github", description: "Issues", transport: "stdio", command: "npx", agents: ["claude"] }],
  hooks: [{ name: "fmt", description: "Formats", event: "after-tool", matcher: "shell", command: "fmt", timeout: 30, agents: ["claude"] }],
  subagents: [{ name: "reviewer", description: "Reviews", prompt: "You review.", skills: [], mcp: [], agents: ["claude"] }],
  foundServers: [], projects: [], foundSkills: [], problems: [],
};

function server(lang, posts) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/library") return json(lib);
    if (url.pathname === "/api/library/export" && req.method() === "POST") {
      posts.push("export");
      // the second export finds one file edited by hand
      const changed = posts.filter((x) => x === "export").length > 1 ? 1 : 0;
      return json({ dir: `${HOME}/.magpie/library/export`, files: 3, changed });
    }
    if (url.pathname === "/api/library/reveal" && req.method() === "POST") {
      posts.push("reveal");
      return json({ dir: `${HOME}/.magpie/library/export` });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}
const L = {
  en: { btn: "Save as files", said: "3 entries written to ~/.magpie/library/export", over: "3 entries written to ~/.magpie/library/export; 1 files edited by hand were written over" },
  zh: { btn: "另存为文件", said: "已把 3 项写入 ~/.magpie/library/export", over: "已把 3 项写入 ~/.magpie/library/export；有 1 个手工改过的文件被覆盖" },
};
const KEYS = ["Save as files", "Write the servers, hooks and subagents as files under the library's export folder, to read and copy", "{n} entries written to {dir}"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the Library writes its entries as files`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 980, height: 760 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", server(lang, posts));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="library"]').click();
      const save = page.locator(".lib-export");
      await save.waitFor();
      assert.equal((await save.textContent()).trim(), w.btn);
      assert.equal(await save.getAttribute("aria-label"), w.btn);
      await save.click();
      await page.waitForFunction((said) => document.querySelector("#status")?.textContent === said, w.said);
      assert.deepEqual(posts, ["export", "reveal"], "written, then opened");
      // a second export says what it wrote over
      await save.click();
      await page.waitForFunction((said) => document.querySelector("#status")?.textContent === said, w.over);
      assert.deepEqual(posts, ["export", "reveal", "export", "reveal"]);
      const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every word has its Chinese");
      assert.deepEqual(errors, []);
      await page.context().close();
    });
  }
}
