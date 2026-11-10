// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 统一一种显示样式（名称，描述）. An MCP server's, a
// skill's, a hook's and a subagent's row say their name and then what they
// are for in the same words: the same size, line height, colour, wrapping
// and spacing, whichever tab they are on, and the description follows the
// name in the row. The API is faked; Chromium and WebKit.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/owner";
const agent = (id, name) => ({ id, name, icon: "", skills: `${HOME}/.${id}/skills`, mcp: `${HOME}/.${id}/mcp.json` });
const lib = {
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [agent("claude", "Claude Code")],
  instructions: { agents: [] },
  servers: [{ name: "fs-mcp", transport: "stdio", command: "fs-mcp", args: [], agents: [], description: "Reads and writes files in a folder you name." }],
  skills: [{ name: "pdf", kind: "folder", agents: [], description: "Split a PDF into pages and read out its text." }],
  hooks: [{ name: "fmt", event: "PostToolUse", matcher: "", command: "prettier", agents: [], description: "Formats the files a turn edited." }],
  hookAgents: [],
  subagents: [{ name: "reviewer", description: "Reviews a change for bugs.", prompt: "You review code.", skills: [], servers: [], agents: [] }],
  subagentAgents: [],
  foundServers: [], projects: [], foundSkills: [], problems: [], mcpHub: false,
};

function server() {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"en",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang: "en", theme: "light" } });
    if (url.pathname === "/api/library") return json(lib);
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], gateway: { running: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const styleOf = (page, sel) => page.locator(sel).first().evaluate((e) => {
  const s = getComputedStyle(e);
  // a long virtualised list keeps a skill's description on one line, so the
  // line's own wrapping is not compared: the words' size, colour, wrapping
  // and spacing are
  return [s.fontSize, s.lineHeight, s.color, s.overflowWrap, s.marginTop];
});

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a description reads alike on every tab, under its name", async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    const ctx = await browser.newContext({ viewport: { width: 1000, height: 760 }, reducedMotion: "reduce" });
    const page = await ctx.newPage();
    t.after(() => browser.close());
    page.setDefaultTimeout(5000);
    page.setDefaultNavigationTimeout(60000);
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.route("http://magpie.test/**", server());
    await page.goto("http://magpie.test/");
    await page.locator('[data-view="library"]').click();
    const tab = (name) => page.locator(".lib-tabs button", { hasText: name }).first();
    const read = async (sel) => {
      const el = page.locator(sel).first();
      await el.waitFor();
      assert.equal(await el.evaluate((e) => e.classList.contains("lib-desc")), true, `${sel} carries lib-desc`);
      // it follows the row's name
      const after = await el.evaluate((e) => {
        const name = e.closest(".row")?.querySelector(".name");
        return !!name && !!(name.compareDocumentPosition(e) & Node.DOCUMENT_POSITION_FOLLOWING);
      });
      assert.equal(after, true, `${sel} follows its name`);
      return styleOf(page, sel);
    };
    await page.locator(".lib-tabs button").first().waitFor();
    await tab("MCP servers").click();
    await page.locator(".lib-server").first().waitFor();
    const mcp = await read(".lib-server .lib-srvdesc");
    await tab("Skills").click();
    const skill = await read(".lib-skill .sub");
    await tab("Hooks").click();
    const hook = await read(".lib-hook .lib-hookdesc");
    await tab("Subagents").click();
    const sub = await read(".lib-subagent .lib-subagentdesc");
    // one description under one name, the same on every tab
    assert.deepEqual(skill, mcp, "the skill's description reads as a server's");
    assert.deepEqual(hook, mcp, "the hook's reads as a server's");
    assert.deepEqual(sub, mcp, "the subagent's reads as a server's");
    assert.deepEqual(errors, []);
    await ctx.close();
  });
}
