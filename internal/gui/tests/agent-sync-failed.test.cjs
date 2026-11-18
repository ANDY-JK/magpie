// Run with Node's test runner and Playwright on the module path; see README.md.
// The review's B-02: a model list magpie couldn't write for an agent is
// said on its row — an amber "List not written" pill after the name, the
// error on hover — where only the log had it. An agent whose sync wrote
// last says nothing. In English and Chinese; no backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const err = "open /fixture/pi/models.json: permission denied";
const fresh = () => ({
  agents: [
    {
      id: "pi", name: "Pi", icon: "generic", path: "/fixture/pi", wired: true,
      fields: [{ key: "model", label: "model", value: "relay/m1", options: [{ value: "relay/m1", label: "m1", ref: "relay/m1" }] }],
      syncError: err,
    },
    {
      id: "codex", name: "Codex", icon: "generic", path: "/fixture/codex",
      fields: [{ key: "model", label: "model", value: "gpt-5.4", options: [{ value: "gpt-5.4", label: "GPT-5.4" }] }],
    },
  ],
  profiles: [],
});

function server(lang) {
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ ...fresh(), settings: { lang, theme: "light" } });
    if (url.pathname === "/api/agents/cli") return json({ agents: {}, pending: false });
    if (url.pathname === "/api/usage/quotas") return json([]);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/plugins") return json({ plugins: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], presets: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const words = {
  en: { pill: "List not written", title: ["couldn't write this agent's model list", err] },
  zh: { pill: "清单未写入", title: ["没能写入这个 agent 的模型清单", err] },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = words[lang];
    test(`${engine} ${lang}: an agent whose list wasn't written says so`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      t.after(() => browser.close());
      const page = await (await browser.newContext({ viewport: { width: 980, height: 520 }, reducedMotion: "reduce" })).newPage();
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", server(lang));
      await page.goto("http://magpie.test/?view=agents");
      const row = page.locator('.row.agent[data-id="pi"]');
      await row.waitFor();
      const pill = row.locator(".badge.ag-sync-failed");
      await pill.waitFor();
      assert.equal((await pill.textContent()).trim(), w.pill);
      const title = await pill.getAttribute("title");
      for (const bit of w.title) assert(title.includes(bit), `the hover says ${JSON.stringify(title)}, missing ${JSON.stringify(bit)}`);
      // an agent whose sync wrote last says nothing
      const clean = page.locator('.row.agent[data-id="codex"]');
      await clean.waitFor();
      assert.equal(await clean.locator(".badge.ag-sync-failed").count(), 0, "a written list says no pill");
      assert.deepEqual(errors, []);
    });
  }
}
