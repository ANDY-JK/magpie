// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): an MCP server says in its own words what it is for, as a
// hook's description (#2011) says for a hook. Its row shows the words under
// its name, with the command or address still under them, muted; its editor
// has a Description field, saved with the server, and nothing is shown for
// one without. At 440 and 1100 wide in en, zh, zh-TW, ja and de: no sideways
// scroll. No backend: the API is faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/owner";
const LONG = "Reads and writes files in a folder you name, and lists what is there: a repository, a workspace or one directory. Nothing outside the folder is touched.";
const agent = (id, name) => ({ id, name, icon: "", skills: `${HOME}/.${id}/skills`, mcp: `${HOME}/.${id}/mcp.json` });
const srv = (name, command, description, agents = [], args = []) => ({ name, transport: "stdio", command, args, agents, ...(description ? { description } : {}) });
const lib = (servers) => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [agent("claude", "Claude Code")],
  instructions: { agents: [] }, servers, skills: [], foundServers: [], projects: [], foundSkills: [], problems: [],
  hooks: [], hookAgents: [], mcpHub: false,
});

let I18N;
const loadI18N = () => { I18N = strings().I18N; };
// the words the page says in lang; a string missing from a language fails
function tr(lang, key, vars = {}) {
  let s = key;
  if (lang !== "en") {
    assert.equal(typeof I18N[lang][key], "string", `${lang} has no ${JSON.stringify(key)}`);
    s = I18N[lang][key];
  }
  return s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
}

function server(lang, state, posts) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/library") return json(lib(state.servers));
    if (url.pathname === "/api/library/market/servers") return json({ items: [] });
    if (url.pathname.startsWith("/api/library/") && req.method() === "POST") {
      const body = req.postDataJSON(), what = url.pathname.replace("/api/library/", "");
      posts.push({ what, body });
      if (what === "servers/save") state.servers = [...state.servers.filter((x) => x.name !== body.old), body.server];
      return json({ ...lib(state.servers), result: { changed: ["claude"] } });
    }
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], gateway: { running: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    if (url.host !== "magpie.test") return route.fulfill({ status: 404, body: "" });
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const settled = (page) => page.waitForFunction(() => !document.querySelector("#modal").getAnimations({ subtree: true }).length);
const sideways = (page) => page.evaluate(() => {
  const over = [], w = document.documentElement.clientWidth;
  if (document.documentElement.scrollWidth > w) over.push("page " + document.documentElement.scrollWidth);
  const v = document.querySelector("#view-library");
  if (v.scrollWidth > v.clientWidth + 1) over.push("library " + v.scrollWidth + ">" + v.clientWidth);
  for (const e of document.querySelectorAll("#view-library .lib-server, #modal .lib-editor *")) {
    const r = e.getBoundingClientRect();
    if (r.width && (r.right > w + 0.5 || r.left < -0.5)) over.push((e.className || e.tagName) + " " + Math.round(r.left) + "–" + Math.round(r.right));
  }
  return over;
});

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": an MCP server's description", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    loadI18N();
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || "chromium" }));
    t.after(() => browser.close());

    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
      await t.test(lang, async () => {
        const w = (key, vars) => tr(lang, key, vars);
        const state = { servers: [srv("files", "fs-mcp", LONG, [], ["--root", "~/code"]), srv("bark", "curl", undefined, [], ["-s", "https://api.day.app/KEY/done"])] };
        const posts = [], errors = [];
        const ctx = await browser.newContext({ viewport: { width: 440, height: 1000 }, reducedMotion: "reduce" });
        await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "mcp"); } catch {} });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        page.on("pageerror", (e) => errors.push(e.message));
        page.on("dialog", (d) => { errors.push("a browser dialog: " + d.message()); d.dismiss(); });
        await page.route("**/*", server(lang, state, posts));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="library"]').click();
        const v = page.locator("#view-library");
        await v.locator(".lib-server").first().waitFor();
        const row = (name) => v.locator(".lib-server").filter({ has: page.locator(".name", { hasText: new RegExp("^" + name + "$") }) });

        // the words are under its name; one without says nothing
        assert.equal((await row("files").locator(".lib-srvdesc").textContent()).trim(), LONG);
        assert.equal(await row("bark").locator(".lib-srvdesc").count(), 0);
        // the command stays under them, muted
        assert.equal((await row("files").locator(".mono").last().textContent()).trim(), "fs-mcp --root ~/code");
        // the whole of a long one is on the screen, wrapped, at 440
        const fit = await row("files").locator(".lib-srvdesc").evaluate((e) => ({
          clipped: e.scrollWidth > e.clientWidth + 1 || e.scrollHeight > e.clientHeight + 1,
          lines: Math.round(e.getBoundingClientRect().height / parseFloat(getComputedStyle(e).lineHeight)),
        }));
        assert.equal(fit.clipped, false, "the description is cut off");
        assert(fit.lines >= 2, `the description isn't wrapped: ${fit.lines} line`);
        assert.deepEqual(await sideways(page), [], "sideways scroll with the list");

        // the editor has its Description, filled, with the library's own words
        await row("files").locator(".name").click();
        const ed = page.locator("#modal .lib-editor");
        await ed.waitFor();
        await settled(page);
        const desc = ed.locator(".lib-srvdescin");
        assert.equal(await desc.inputValue(), LONG);
        assert.equal(await desc.getAttribute("maxlength"), "200");
        await ed.locator(".hint", { hasText: w("Optional: what it's for, in any language. Only the library keeps it") }).first().waitFor();
        assert.equal(await ed.locator(".hint", { hasText: w("Optional: what it's for, in any language. Only the library keeps it") }).first().textContent(), w("Optional: what it's for, in any language. Only the library keeps it"));
        await desc.fill("Reads a repository's files");
        assert.deepEqual(await sideways(page), [], "sideways scroll in the editor");
        await ed.locator(".bar button.primary").click();
        await ed.waitFor({ state: "detached" });
        assert.deepEqual(posts.shift(), {
          what: "servers/save",
          body: { old: "files", server: { name: "files", description: "Reads a repository's files", transport: "stdio", agents: [], startupTimeout: 0, local: false, hub: "", command: "fs-mcp", args: ["--root", "~/code"], env: {} } },
        });
        await page.waitForFunction(() => document.querySelector("#view-library .lib-server .lib-srvdesc")?.textContent.trim() === "Reads a repository's files");
        assert.equal((await row("files").locator(".lib-srvdesc").textContent()).trim(), "Reads a repository's files");

        // a server added without one has none, and says nothing on its row
        await v.locator(".after-list button").first().click();
        const add = page.locator("#modal .lib-editor");
        await add.waitFor();
        await settled(page);
        assert.equal(await add.locator(".lib-srvdescin").inputValue(), "");
        await add.locator("input").first().fill("notes");
        await add.locator("input.mono").first().fill("notes-mcp");
        await add.locator(".bar button.primary").click();
        await add.waitFor({ state: "detached" });
        const saved = posts.shift();
        assert.equal(saved.what, "servers/save");
        assert.equal(saved.body.server.description, "");
        assert.equal(await row("notes").locator(".lib-srvdesc").count(), 0);

        // 1100 wide: the description is whole and the row doesn't scroll
        const wctx = await browser.newContext({ viewport: { width: 1100, height: 800 }, reducedMotion: "reduce" });
        await wctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "mcp"); } catch {} });
        const wide = await wctx.newPage();
        wide.setDefaultTimeout(5000);
        wide.setDefaultNavigationTimeout(60000);
        wide.on("pageerror", (e) => errors.push(e.message));
        await wide.route("**/*", server(lang, state, []));
        await wide.goto("http://magpie.test/");
        await wide.locator('button[data-view="library"]').click();
        const wr = wide.locator("#view-library .lib-server").filter({ has: wide.locator(".name", { hasText: /^files$/ }) });
        await wr.locator(".lib-srvdesc").waitFor();
        assert.equal(await wr.locator(".lib-srvdesc").evaluate((e) => e.scrollWidth > e.clientWidth + 1 || e.scrollHeight > e.clientHeight + 1), false, "the description is cut off at 1100");
        await wctx.close();

        // a server given to an agent is checked once it is on the page; the
        // saves are what must have ended here
        assert.deepEqual(posts.filter((p) => p.what === "servers/save"), []);
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
}
