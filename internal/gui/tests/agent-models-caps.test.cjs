// Run with Node's test runner and Playwright on the module path; see README.md.
// What Test capabilities settled of a model is kept with the provider
// (providers.json), and the Agents page's model list says it on the model's
// row: T tools, V vision and S streaming, green for the endpoint's yes and
// red for its no, the words on hover — what the editor's chip says. A model
// not tested says no letter. English and Chinese, both engines; no backend,
// the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const CAPS = { tools: "yes", vision: "no", stream: "yes" };

function fixture(lang) {
  const list = [
    { id: "relay/m1", name: "M1", group: "Relay", icon: "generic", context: 128000, inUse: true, caps: CAPS },
    { id: "relay/m2", name: "M2", group: "Relay", icon: "generic", context: 32000 },
  ];
  const count = () => ({ shown: 2, listed: 2, by: [{ name: "Relay", icon: "generic", n: 2 }] });
  const state = () => ({
    agents: [{
      id: "claude", name: "Claude Code", path: "/test/settings.json", icon: "claude-color", wired: true,
      fields: [{ key: "model", label: "model", value: "relay/m1", options: [] }],
      models: count(),
    }],
    profiles: [], settings: { lang, theme: "light" },
  });
  async function serve(route) {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state());
    if (url.pathname === "/api/agent-models/claude") return json({ models: list });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], gateway: { running: true, window: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  }
  return { serve };
}

const L = {
  en: {
    tools: "Tools: it called the tool it was asked to", noVision: "Vision: it turned an image down",
    stream: "Streaming: it answered as a stream",
  },
  zh: {
    tools: "工具：它按请求调用了工具", noVision: "视觉：它拒绝了图片",
    stream: "流式：它用流式回答",
  },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: the model list says what a model can do, when it was tested`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 1100, height: 700 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const fx = fixture(lang);
      await page.route("**/*", fx.serve);
      await page.goto("http://magpie.test/");

      const row = page.locator('.row.agent[data-id="claude"]');
      await row.locator(".ag-link").click();
      await row.locator(".ag-exp .ag-chips .ag-quiet").click();
      const pop = page.locator(".am-pop:not(.leaving)");
      await pop.waitFor();
      const mr = (name) => pop.locator(".am-mr", { hasText: name });
      await mr("M1").waitFor();
      // the tested model's row: a letter each, green for yes and red for no,
      // the words on hover
      const badges = await mr("M1").locator(".badge.cap").evaluateAll((bs) => bs.map((b) => ({ text: b.textContent, cls: b.className, title: b.title })));
      assert.deepEqual(badges, [
        { text: "T", cls: "badge cap ok", title: w.tools },
        { text: "V", cls: "badge cap bad", title: w.noVision },
        { text: "S", cls: "badge cap ok", title: w.stream },
      ]);
      // a model not tested says none
      assert.deepEqual(await mr("M2").locator(".badge.cap").count(), 0, "an untested model has no letters");
      assert.deepEqual(errors, []);
    });
  }
}
