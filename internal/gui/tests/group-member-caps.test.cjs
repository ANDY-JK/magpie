// Run with Node's test runner and Playwright on the module path; see README.md.
// What Test capabilities settled of a member's model is kept with the
// provider (providers.json), and the Routing page's group editor says it on
// the member's row: T tools, V vision and S streaming, green for the
// endpoint's yes and red for its no, the words on hover — what the editor's
// chip says. A member not tested, and one that is itself a group, says no
// letter. English and Chinese, both engines; no backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const CAPS = { tools: "yes", vision: "no" };

function fixture(lang) {
  const models = [
    { id: "relay/m1", name: "m1", providerName: "Relay", icon: "generic", context: 128000 },
    { id: "relay/m2", name: "m2", providerName: "Relay", icon: "generic", context: 32000 },
  ];
  const members = ["relay/m1", "relay/m2"];
  const groups = () => ({
    models, pools: [],
    groups: [{ id: "sol", name: "Sol", members, routing: "order", ready: true,
      memberInfo: [
        { id: "relay/m1", ready: true, provider: "relay", name: "Relay", context: 128000, caps: CAPS },
        { id: "relay/m2", ready: true, provider: "relay", name: "Relay", context: 32000 },
      ] }],
  });
  async function serve(route) {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [{ id: "codex", name: "Codex", path: "/test/codex", fields: [] }], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/gateway/trace") {
      if (url.searchParams.get("wait")) return new Promise(() => {});
      return json({ mine: true, now: new Date().toISOString(), seq: 1, totals: { requests: 0, rerouted: 0, errors: 0 }, routes: [] });
    }
    if (url.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (url.pathname === "/api/groups") return json(groups());
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  }
  return { serve };
}

const L = {
  en: {
    edit: "Edit",
    tools: "Tools: it called the tool it was asked to", noVision: "Vision: it turned an image down",
  },
  zh: {
    edit: "编辑",
    tools: "工具：它按请求调用了工具", noVision: "视觉：它拒绝了图片",
  },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: a member's row says what its model can do, when it was tested`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1100, height: 2000 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const fx = fixture(lang);
      await page.route("**/*", fx.serve);
      await page.goto("http://magpie.test/?view=routing");
      const card = page.locator(".rt-group", { hasText: "Sol" });
      await card.waitFor();
      await card.locator("button", { hasText: w.edit }).click();
      const ed = page.locator(".rt-gedit");
      const row = (name) => ed.locator(".fbrow", { hasText: name });
      await row("m1").waitFor();
      // the tested member's row: a letter each, green for yes and red for
      // no, the words on hover
      const badges = await row("m1").locator(".badge.cap").evaluateAll((bs) => bs.map((b) => ({ text: b.textContent, cls: b.className, title: b.title })));
      assert.deepEqual(badges, [
        { text: "T", cls: "badge cap ok", title: w.tools },
        { text: "V", cls: "badge cap bad", title: w.noVision },
      ]);
      // a member not tested says none
      assert.deepEqual(await row("m2").locator(".badge.cap").count(), 0, "an untested member has no letters");
      assert.deepEqual(errors, []);
    });
  }
}
