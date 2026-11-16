// Run with Node's test runner and Playwright on the module path; see README.md.
// A plugin that gives tools is a server of the library's own
// (plugin_tools.go): its row says which plugin it comes from, with a word
// on what that means — it comes and goes with the plugin — while a server
// of the user's own says nothing. The header has a Plugins button that
// opens the Plugins page, where the plugin is installed or removed.
// In English and Chinese, and the four languages' words are checked
// against strings.cjs. No backend: the API is faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/lc";
const TAG = "From the plugin {name}";
const WHY = "The plugin serves it: it comes and goes with the plugin, and is installed or removed on the Plugins page";
// the plugin's tools as the library holds one: named for the plugin's
// short name, at the gateway's path for it, kept to this computer
const pluginServer = {
  name: "search1api", transport: "http", url: "http://127.0.0.1:3989/mcp/magpie/tools/search1api",
  plugin: "search1api", local: true, agents: [],
};
const fixture = () => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [{ id: "claude", name: "Claude Code", icon: "claudecode-color", skills: `${HOME}/.claude/skills`, mcp: `${HOME}/.claude.json`, projectMCP: ".mcp.json" }],
  instructions: { agents: [], sets: [] }, foundServers: [], foundSkills: [], skills: [], projects: [],
  servers: [pluginServer, { name: "files", transport: "stdio", command: "npx", args: ["files-mcp"], agents: [] }],
});

function serve(lang) {
  return async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang } } });
    if (url.pathname === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
    if (url.pathname === "/api/library") return route.fulfill({ json: fixture() });
    if (url.pathname === "/api/library/mcp/check") return route.fulfill({ json: { servers: {} } });
    if (url.pathname === "/api/usage/quotas") return route.fulfill({ json: [] });
    if (url.pathname === "/api/groups") return route.fulfill({ json: { groups: [] } });
    if (url.pathname === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ json: {} });
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const words = {
  en: { tag: "From the plugin search1api", why: WHY, plugins: "Plugins" },
  zh: { tag: "来自插件 search1api", why: "由该插件提供：随插件生灭，装与卸都在插件页", plugins: "插件" },
};

// the four languages carry the page's new words, and none of them re-says
// one the window already has ("Plugins" among them)
test("the four languages carry the plugin server's words", () => {
  const s = strings();
  const lib = s.plugins.find((p) => p.id === "library");
  assert.ok(lib, "the library plugin's words are here");
  for (const lang of ["zh", "zh-TW", "ja", "de"]) {
    const d = lib.dict[lang];
    assert.match(d[TAG], /\{name\}/, lang + " fills the plugin's name in");
    assert.equal(typeof d[WHY], "string", lang + " says what the tag means");
    assert.equal("Plugins" in d, false, lang + " re-said a word the window has");
  }
  assert.deepEqual(s.twice.filter((t) => /library/.test(t)), [], "a word said twice in the library plugin's files");
});

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": a plugin's server says so, and the header opens the Plugins page", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    const errors = [];
    t.after(async () => {
      if (errors.length) console.log(errors);
      await browser.close();
    });

    for (const lang of ["en", "zh"]) {
      await t.test(lang, async () => {
        const w = words[lang];
        const ctx = await browser.newContext({ viewport: { width: 980, height: 500 } });
        await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "mcp"); } catch {} });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("http://magpie.test/**", serve(lang));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="library"]').click();
        await page.locator("#view-library .lib-body:not(.lib-skel) .lib-row").first().waitFor();

        // the plugin's row names the plugin; a server of the user's own
        // carries no such tag
        const tag = page.locator("#view-library .lib-server .lib-tag.lib-plugin");
        await tag.waitFor();
        assert.equal(await tag.textContent(), w.tag);
        assert.equal(await tag.getAttribute("title"), w.why);
        const filesRow = page.locator("#view-library .lib-server").filter({ has: page.locator('.lib-health[data-server="files"]') });
        assert.equal(await filesRow.locator(".lib-tag.lib-plugin").count(), 0, "the user's own server says nothing of a plugin");

        // the header's Plugins button opens the Plugins page: app.js's own
        // show, watched here so the click is the only thing judged
        const plg = page.locator("#view-library .lib-headacts .lib-plugins");
        assert.equal(await plg.textContent(), w.plugins);
        assert.equal(await plg.getAttribute("aria-label"), w.plugins);
        await page.evaluate(() => { window.__shownViews = []; window.show = (v) => { window.__shownViews.push(v); return true; }; });
        await plg.click();
        assert.deepEqual(await page.evaluate(() => window.__shownViews), ["plugins"]);
        await ctx.close();
      });
    }
  });
}
