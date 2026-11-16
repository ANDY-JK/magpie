// Run with Node's test runner and Playwright on the module path; see README.md.
// The Library's Skills and Instructions tabs had no way in from the page at
// all: Skills' header now has a Skills folder button and Instructions' an
// Instruction sets folder button, right after Library folder, each opening
// the folder of its own (a folder not made yet opens the folder it would be
// in, which the reveal endpoint takes care of). Its title says what the
// folder is and gives the tilde path whether the folder is there or not. No
// other tab has either button. In every language, in both engines. No
// backend: the API is faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/tester";
const agent = (id, name) => ({ id, name, icon: "", skills: `${HOME}/.${id}/skills`, mcp: `${HOME}/.${id}/mcp.json` });
const view = {
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`,
  // a library never saved as files has no export folder yet, and the skills
  // and instruction sets are named where they would be: the buttons still
  // say so, and open the folder they would be in
  skillsDir: `${HOME}/.magpie/library/skills`, setsDir: `${HOME}/.magpie/library/instruction-sets`,
  home: HOME, agents: [agent("claude", "Claude Code")],
  instructions: { agents: [], sets: [] }, servers: [], foundServers: [], projects: [], foundSkills: [], skills: [], problems: [], hooks: [],
};

function serve(lang, opened) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/library") return json(view);
    if (url.pathname === "/api/library/rtk") return json({ path: `${HOME}/.local/bin/rtk`, version: "0.51.0", url: "https://www.rtk-ai.app", agents: [{ id: "claude", name: "Claude Code", icon: "", on: true }] });
    if (url.pathname === "/api/library/reveal") { opened.push(req.postDataJSON()); return json({}); }
    if (url.pathname === "/api/library/market/servers" || url.pathname === "/api/library/market/skills") return json({ items: [] });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], gateway: { running: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    const body = await fs.readFile(file).catch(() => null);
    await (body ? route.fulfill({ body, contentType }) : route.fulfill({ status: 404, body: "" }));
  };
}

// each tab, and the folder button of its own: where it sits, what it says,
// the path it gives the reveal endpoint — and neither button on any other tab
const SKILLS_WHY = "The skills the library keeps are here, one folder to a skill.";
const SETS_WHY = "The instruction sets besides the first are kept here, one file to a set.";
const TABS = [
  { tab: "instructions", cls: ".lib-setsdir", key: "Instruction sets folder", why: SETS_WHY, dir: view.setsDir },
  { tab: "mcp", cls: null },
  { tab: "skills", cls: ".lib-skillsdir", key: "Skills folder", why: SKILLS_WHY, dir: view.skillsDir },
  { tab: "hooks", cls: null },
  { tab: "rtk", cls: null },
];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) {
    test(`${engine} ${lang}: each tab's folder button opens the folder of its own`, async (t) => {
      const I18N = strings().I18N;
      // every word has its four languages; English is the key itself, which
      // needs no file
      const missing = lang === "en" ? [] : ["Skills folder", SKILLS_WHY, "Instruction sets folder", SETS_WHY].filter((k) => !I18N[lang]?.[k]);
      assert.deepEqual(missing, [], "every word has its translation");
      const word = (k) => (I18N[lang]?.[k] || k);
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      for (const { tab, cls, key, why, dir } of TABS) {
        const where = `${engine} ${lang} ${tab}`;
        const context = await browser.newContext({ viewport: { width: 1000, height: 700 }, reducedMotion: "reduce" });
        await context.addInitScript((tb) => { try { localStorage.setItem("magpie.libTab", tb); } catch {} }, tab);
        const page = await context.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        const errors = [], opened = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", serve(lang, opened));
        await page.goto("http://magpie.test/?view=library");
        const v = page.locator("#view-library");
        const head = v.locator(".lib-head");
        await head.waitFor();
        await v.locator(".lib-body").first().waitFor({ state: "attached" }).catch((e) => { throw new Error(where + ": " + e.message); });
        // Library folder is on every tab; the two folder buttons only on their own
        assert.equal(await head.locator(".lib-more").count(), 1, `${where}: Library folder is on every tab`);
        assert.equal(await head.locator(".lib-skillsdir").count(), tab === "skills" ? 1 : 0, `${where}: the skills' folder button is on the Skills tab alone`);
        assert.equal(await head.locator(".lib-setsdir").count(), tab === "instructions" ? 1 : 0, `${where}: the instruction sets' folder button is on the Instructions tab alone`);
        if (cls) {
          const b = head.locator(cls);
          assert.equal((await b.textContent()).trim(), word(key), `${where}: the button is named`);
          assert.equal(await b.getAttribute("aria-label"), word(key), `${where}: named by its label`);
          assert.equal(await b.isDisabled(), false, `${where}: a folder not made yet is no matter`);
          // what the folder is, and the tilde path, there or not
          assert.equal(await b.getAttribute("title"), word(why) + "\n" + dir.replace(HOME, "~"), `${where}: its title says what the folder is and gives its path under ~`);
          // it opens the folder of its own, and the page stays where it was
          const top = await v.evaluate((e) => e.scrollTop);
          await Promise.all([
            page.waitForResponse((r) => new URL(r.url()).pathname === "/api/library/reveal"),
            b.click(),
          ]);
          assert.deepEqual(opened, [{ path: dir }], `${where}: it opens the folder of its own`);
          assert.equal(await v.evaluate((e) => e.scrollTop), top, `${where}: the click moved the page`);
        }
        assert.deepEqual(errors, [], where);
        await context.close();
      }
    });
  }
}
