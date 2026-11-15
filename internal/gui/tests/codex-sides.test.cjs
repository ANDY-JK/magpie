// Run with Node's test runner and Playwright on the module path; see README.md.
// Codex's side requests are one entry in its opened row (codexSidesRow):
// the six model rows — titles, descriptions, predictions, auto-review,
// memories, subagents — hold in the sheet it opens (openCodexSides), where
// each keeps its own picker and meaning, six pickers in the row read as
// one. The entry counts the sides with a setting of their own. A pick posts
// on its own (settings/codex-titles, or the agent's memories field), Done
// shuts, and reopened the sheet shows what the server kept. No native
// select, and the opened row and the sheet fit 560px with nothing sideways
// (agent-layout's read). English and Chinese, Chromium and WebKit; no
// backend, the API is faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");
const { watchSettingsReads, agentRow, openAgentSettings, pickItem } = require("./reader.cjs");

const assets = path.resolve(__dirname, "../assets");
const SIDES = ["codexTitlesRow", "codexDescriptionsRow", "codexPredictionsRow", "codexAutoReviewRow", "codexMemoriesRow", "codexSubagentModelRow"];

function settingsPayload(over) {
  return {
    theme: "light", lang: "en", tray: "panel", quotaLeft: false, currency: "usd",
    dock: false, dockWindow: false, proxy: "", redact: false, redactPersonal: false, redactWords: [],
    codexWarmup: "", claudeWarmup: "", codexWarmAt: "", claudeWarmAt: "", workbuddyCheckin: false, noStats: false,
    trayUsage: "", trayUsageEvery: 3, vision: "", imageGen: "",
    version: "0.1.400", dir: "~/.config/magpie", gateway: "http://127.0.0.1:3425",
    proxyNow: "none", proxySource: "none", login: false,
    visionModels: [], imageGenModels: [], workbuddyCheckins: [], lanURLs: [],
    titleModels: [
      { id: "deepseek/deepseek-v4-flash", name: "DeepSeek V4 Flash", provider: "deepseek", providerName: "DeepSeek", icon: "deepseek" },
      { id: "group/cheap", name: "cheap", provider: "", providerName: "Routing groups" },
    ],
    fx: { rate: 7.2, at: new Date().toISOString(), stale: false },
    ...over,
  };
}

function server(lang, posted) {
  let cur = settingsPayload({ lang });
  // memories and subagent model are Codex's own fields, shown as rows since
  // they have options to pick from
  const codex = agentRow("codex", "Codex");
  codex.fields.push(
    { key: "memories", label: "memories", value: "", options: [{ value: "gpt-5.5", label: "GPT-5.5", ref: "openai/gpt-5.5" }, { value: "gpt-5.4-mini", label: "GPT-5.4 mini", ref: "openai/gpt-5.4-mini" }] },
    { key: "subagent_model", label: "subagent model", value: "", options: [{ value: "gpt-5.5", label: "GPT-5.5", ref: "openai/gpt-5.5" }] },
  );
  const state = () => ({ agents: [codex, agentRow("claude", "Claude Code"), agentRow("pi", "Pi")], profiles: [], settings: { lang, theme: "light" }, fx: cur.fx });
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:false};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    // agents below Codex, so its opened row is not the list's end
    if (url.pathname === "/api/state") return json(state());
    for (const [path, key] of [["codex-titles", "codexTitles"], ["codex-descriptions", "codexDescriptions"], ["codex-predictions", "codexPredictions"], ["codex-auto-review", "codexAutoReview"]]) {
      if (url.pathname === "/api/settings/" + path) {
        const body = req.postDataJSON();
        posted.push([path, body]);
        cur = { ...cur, [key]: body.model };
        return json(cur);
      }
    }
    if (url.pathname === "/api/set") {
      const body = req.postDataJSON();
      posted.push(["set", body]);
      const f = codex.fields.find((f) => f.key === body.field);
      f.value = body.value;
      return json(state());
    }
    if (url.pathname === "/api/settings") return json(cur);
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [], models: [] });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    try {
      await route.fulfill({ body: await fs.readFile(file), contentType });
    } catch {
      await route.fulfill({ status: 404, body: "" });
    }
  };
}

const want = {
  en: { head: "Codex side requests", none: "None set", set: (n, all) => `${n} of ${all} set`, off: "Off", own: "Same as the session" },
  zh: { head: "Codex 附属请求", none: "都未设置", set: (n, all) => `${all} 项里已设 ${n} 项`, off: "关闭", own: "跟随当前会话" },
};
const sheet = (page) => page.locator("#modal:not([hidden]) .editor.agent-prefs");
const shownIn = (page, box) => page.locator(box).evaluate((b) => [...b.querySelectorAll(":scope [id$=Row]")].filter((r) => r.checkVisibility()).map((r) => r.id));

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of Object.keys(want)) {
    test(`${engine} ${lang}: Codex's side requests are one entry, each pickable in its sheet`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await browser.newPage({ viewport: { width: 560, height: 760 }, reducedMotion: "reduce" });
      page.setDefaultTimeout(8000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], posted = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang, posted));
      t.after(async () => {
        if (process.env.ARTIFACT_DIR) {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `${engine}-${lang}-codex-sides.png`) });
        }
        await browser.close();
      });
      await watchSettingsReads(page);
      await page.goto("http://magpie.test/?view=agents");
      await openAgentSettings(page);
      const w = want[lang];
      const rowBox = '.row.agent[data-id="codex"] .ag-prefs';
      const entry = page.locator("#codexSidesRow button");
      await entry.waitFor();

      // the entry alone, the six rows nowhere in the opened row
      assert.equal((await entry.textContent()).trim(), w.none, "nothing set yet");
      for (const id of SIDES) assert.equal(await page.locator(`${rowBox} #${id}`).count(), 0, `${id} not in the opened row`);
      assert.equal(await page.locator("#codexSidesRow select").count(), 0, "no native select");
      // the opened row and the page fit 560px (agent-layout's read)
      assert(await page.locator(rowBox).evaluate((b) => b.scrollWidth <= b.clientWidth + 1), "the opened row's settings fit 560px");
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "the page fits 560px");

      // the entry opens the sheet, the six in it
      await entry.click();
      await sheet(page).waitFor();
      assert.equal((await sheet(page).locator(".ehead").textContent()).trim(), w.head);
      assert.deepEqual(await shownIn(page, "#modal .editor.agent-prefs"), SIDES, "the six rows in the sheet");
      assert(await page.locator("#modal .dialog").evaluate((d) => d.scrollWidth <= d.clientWidth + 1), "the sheet fits 560px");
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), "no sideways scroll under the sheet");
      const titles = page.locator("#codexTitlesPick button");
      assert.equal((await titles.textContent()).trim(), w.own, "the session's by default");
      assert.equal(await sheet(page).locator("select").count(), 0, "no native select in the sheet");

      // titles to Off: posted on its own, the entry counts it
      await titles.click();
      await page.locator("#pop:not([hidden]) #list li").first().waitFor();
      await pickItem(page, page.locator("#list li:not(.group)").filter({ hasText: w.off }).first());
      await page.locator("#codexTitlesPick button", { hasText: w.off }).waitFor();
      assert.deepEqual(posted, [["codex-titles", { model: "off" }]]);
      assert.equal((await entry.textContent()).trim(), w.set(1, 6), "the entry counts the one set");

      // memories to a model of magpie's: posted as the field it is
      const memories = page.locator("#codexMemoriesPick button");
      await memories.click();
      await page.locator("#pop:not([hidden]) #list li").first().waitFor();
      await pickItem(page, page.locator("#list li:not(.group)").filter({ hasText: "GPT-5.4 mini" }).first());
      await page.locator("#codexMemoriesPick button", { hasText: "GPT-5.4 mini" }).waitFor();
      assert.deepEqual(posted.at(-1), ["set", { agent: "codex", field: "memories", value: "gpt-5.4-mini" }]);
      assert.equal((await entry.textContent()).trim(), w.set(2, 6), "two set");

      // Done shuts; the rows are back in the page, the count kept
      await sheet(page).locator(".bar .primary").click();
      await page.locator("#modal").waitFor({ state: "hidden" });
      await entry.waitFor();
      assert.equal((await entry.textContent()).trim(), w.set(2, 6), "the count kept after Done");
      for (const id of SIDES) assert.equal(await page.locator(`#${id}`).count(), 1, `${id} back in the page`);

      // reopened, the sheet shows what the server kept
      await entry.click();
      await sheet(page).waitFor();
      assert.equal((await page.locator("#codexTitlesPick button").textContent()).trim(), w.off, "the pick kept");
      assert.equal((await page.locator("#codexMemoriesPick button").textContent()).trim(), "GPT-5.4 mini", "the field's pick kept");
      assert.deepEqual(await shownIn(page, "#modal .editor.agent-prefs"), SIDES, "the six again");
      await page.keyboard.press("Escape");
      await page.locator("#modal").waitFor({ state: "hidden" });
      for (const id of SIDES) assert.equal(await page.locator(`#${id}`).count(), 1, `${id} back once shut`);
      assert.deepEqual(errors, []);
    });
  }
}
