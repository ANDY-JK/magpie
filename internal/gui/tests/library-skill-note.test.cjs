// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 技能描述要转为中文记录，不改原文. A skill's page (its
// SKILL.md reader) now carries a Chinese description field: what the user
// writes is kept in magpie's library on this computer, the skill's SKILL.md
// untouched, and shown under the skill's name in place of its English
// description, which stays in the hover. The row says 中文 once it has one.
// English and Chinese, both engines; no backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { strings } = require("./strings.cjs");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/loosheng";
const EN = "Split a PDF into pages and read out its text.";
const NOTE = "把 PDF 拆成页，并读出文字。";
const agent = (id, name) => ({ id, name, icon: id, skills: `${HOME}/.${id}/skills`, mcp: `${HOME}/.${id}/mcp.json` });
const lib = (note) => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [agent("claude", "Claude Code"), agent("codex", "Codex")],
  instructions: { agents: [], sets: [] }, servers: [], foundServers: [], projects: [], foundSkills: [],
  skills: [{ name: "pdf", kind: "folder", agents: ["claude"], description: EN, ...(note ? { note } : {}) }],
});

function server(lang, theme, state, posts) {
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"${theme}",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme } } });
    if (url.pathname === "/api/library") return route.fulfill({ json: { ...lib(state.note), problems: [] } });
    if (url.pathname === "/api/library/skills/note") {
      const body = req.postDataJSON();
      posts.push(body);
      state.note = body.note.trim().replace(/\s+/g, " ");
      return route.fulfill({ json: { ...lib(state.note), result: { changed: [], problems: [] } } });
    }
    if (url.pathname === "/api/library/skill") return route.fulfill({ json: { text: "# PDF\n\nSplit it.", path: `${HOME}/.magpie/library/pdf/SKILL.md` } });
    if (url.pathname === "/api/usage/quotas") return route.fulfill({ json: [] });
    if (url.pathname === "/api/groups") return route.fulfill({ json: { groups: [] } });
    if (url.pathname === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ json: {} });
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try { await route.fulfill({ body: await fs.readFile(file), contentType }); } catch { await route.fulfill({ status: 404, body: "" }); }
  };
}

const L = {
  en: { desc: "Chinese description", save: "Save description", tag: "Chinese", hint: "Kept in magpie's library on this computer; the skill's SKILL.md is not changed.", saved: "The Chinese description is saved" },
  zh: { desc: "中文说明", save: "保存说明", tag: "中文", hint: "只保存在这台电脑的 magpie 资源库里；技能自己的 SKILL.md 不会被改动。", saved: "中文说明已保存" },
};
const KEYS = ["Chinese description", "e.g. Split a PDF into pages and read out its text", "Kept in magpie's library on this computer; the skill's SKILL.md is not changed.",
  "Its own description, from SKILL.md", "Save description", "The Chinese description is saved", "Chinese",
  "A Chinese description you saved for this skill, shown in place of its own"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: a skill's Chinese description is kept in magpie's library`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const ctx = await browser.newContext({ viewport: { width: 1000, height: 760 }, reducedMotion: "reduce" });
      await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "skills"); } catch {} });
      const page = await ctx.newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posts = [], state = { note: "" };
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("http://magpie.test/**", server(lang, "light", state, posts));
      await page.goto("http://magpie.test/");
      await page.locator('[data-view="library"]').click();
      const row = page.locator(".lib-skill").first();
      await row.waitFor();
      assert.equal((await row.locator(".sub").textContent()).trim(), EN);
      assert.equal(await row.locator(".lib-notezh").count(), 0, "no 中文 tag before a note");

      // the skill's page: its own description, and a field for the Chinese one
      await row.click();
      const ed = page.locator("#modal .editor");
      await ed.waitFor();
      const ta = ed.locator(".lib-notein");
      assert.equal(await ta.inputValue(), "");
      assert.equal((await ed.locator(".lib-noteorig").textContent()).trim(), EN, "the original description is shown");
      assert.equal((await ed.locator(".hint").first().textContent()).trim(), w.hint);
      await ta.fill(NOTE);
      await ed.getByRole("button", { name: w.save, exact: true }).click();
      for (let i = 0; i < 60 && !posts.length; i++) await page.waitForTimeout(50);
      assert.deepEqual(posts, [{ name: "pdf", note: NOTE }]);
      await ed.waitFor({ state: "detached" });
      // the row says it in Chinese, its own description still in the hover
      await page.waitForFunction((want) => document.querySelector(".lib-skill .sub")?.textContent.trim() === want, NOTE);
      assert.equal((await row.locator(".sub").textContent()).trim(), NOTE);
      assert((await row.locator(".sub").getAttribute("title")).includes(EN), "the SKILL.md's description stays in the hover");
      assert.equal((await row.locator(".lib-notezh").textContent()).trim(), w.tag);

      // kept: a reload of the page reads it from the library
      await page.reload();
      await page.locator('[data-view="library"]').click();
      const again = page.locator(".lib-skill").first();
      await again.waitFor();
      await page.waitForFunction((want) => document.querySelector(".lib-skill .sub")?.textContent.trim() === want, NOTE);

      const missing = KEYS.filter((k) => !strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every new word has its Chinese");
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  }
}
