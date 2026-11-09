// Run with Node's test runner and Playwright on the module path; see README.md.
// Moving the picker's highlight rebuilt the whole list per row the pointer
// crossed (renderList on every mousemove, and again per arrow key): a long
// model list was re-laid for nothing each time. The cursor now turns over
// only the two rows' sel class (setCursor); a full rebuild stays with the
// filter and the fold. Probed with a property a rebuild drops: the rows the
// pointer crossed must still be the same nodes, the sel on the row the
// pointer or the keys are at, and Enter commits the row the keys ended on.
// In English; Chromium and WebKit. No backend: the API is faked here, and
// /api/model-speeds is held, since its answer redraws the list once after
// the picker opens and would mask the hover under test.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(process.env.MAGPIE_GUI_ASSETS || path.resolve(__dirname, "../assets"));
// a list long enough to scroll: the rebuild this asks about is one a reader
// scrolling it would pay per row crossed
const models = Array.from({ length: 40 }, (_, i) => ({ value: "model-" + i, ref: "model-" + i, label: "Model " + i }));
const agent = {
  id: "claude", name: "Claude Code", path: "/test/claude", wired: true, // on magpie models (#726: a connected one is in view)
  fields: [{ key: "model", label: "model", value: "model-0", options: models }],
};
const fresh = () => ({ agents: [agent], profiles: [] });

function server(lang, sets) {
  let cur = fresh();
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return route.fulfill({ json: { ...cur, settings: { lang, theme: "light" } } });
    if (url.pathname === "/api/set") {
      const body = req.postDataJSON();
      sets.push(body);
      cur = JSON.parse(JSON.stringify(cur));
      cur.agents.find((a) => a.id === body.agent).fields.find((f) => f.key === body.field).value = body.value;
      return route.fulfill({ json: { ...cur, settings: { lang, theme: "light" } } }).catch(() => {});
    }
    if (url.pathname === "/api/model-speeds") return new Promise(() => {}); // stays waiting: its redraw would rebuild the list
    if (url.pathname === "/api/usage/quotas") return route.fulfill({ json: [] });
    if (url.pathname === "/api/groups") return route.fulfill({ json: { groups: [] } });
    if (url.pathname === "/api/agents/cli") return route.fulfill({ json: { agents: {}, pending: false } });
    if (url.pathname === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ json: {} });
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const row = '.row.agent[data-id="claude"]';
// mark: a property on the row's node, dropped by a rebuild alone
const mark = (page, i) => page.evaluate((i) => { document.querySelector(`#list li[data-i="${i}"]`).probe = "keep"; }, i);
const marked = (page, i) => page.evaluate((i) => document.querySelector(`#list li[data-i="${i}"]`)?.probe === "keep", i);
const sel = (page) => page.evaluate(() => document.querySelector("#list li.sel")?.dataset.i ?? null);

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": hover and the keys move the highlight without rebuilding the list", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    const errors = [];
    t.after(async () => {
      if (errors.length) console.log(errors);
      await browser.close();
    });
    const sets = [];
    const page = await (await browser.newContext({ viewport: { width: 980, height: 720 } })).newPage();
    page.setDefaultTimeout(5000);
    page.setDefaultNavigationTimeout(60000);
    page.on("pageerror", (e) => errors.push(e.message));
    await page.route("**/*", server("en", sets));
    await page.goto("http://magpie.test/");
    await page.locator(row).waitFor();

    await page.locator(`${row} > .field.ag-start[data-key="model"]`).click();
    await page.locator("#pop:not([hidden]) #list li").first().waitFor();

    // the pointer crosses to row 5: the highlight follows it, the row it
    // left loses it, and the rows are the same nodes
    await mark(page, 2);
    await page.locator('#list li[data-i="5"]').hover();
    assert.equal(await sel(page), "5", "the highlight followed the pointer");
    assert(!(await page.evaluate(() => document.querySelector('#list li[data-i="0"]').classList.contains("sel"))), "the row the pointer left");
    assert(await marked(page, 2), "the crossed row was rebuilt");

    // back to row 2, then the keys: the same rows, the sel with the keys
    await page.locator('#list li[data-i="2"]').hover();
    assert.equal(await sel(page), "2");
    assert(await marked(page, 2), "still the same node after hovering back");
    await page.keyboard.press("ArrowDown");
    assert.equal(await sel(page), "3", "the keys moved the highlight");
    assert(await marked(page, 2), "the arrow keys rebuilt the list");

    // Enter commits the row the keys ended on (the list opens with the
    // reset row and Disconnect from magpie, so row 3 is Model 1)
    assert.equal(await page.locator('#list li[data-i="3"] .v').textContent(), "Model 1");
    await page.keyboard.press("Enter");
    await page.waitForFunction(() => document.querySelector("#pop").hidden);
    assert.deepEqual(sets, [{ agent: "claude", field: "model", value: "model-1" }], JSON.stringify(sets));
    await page.context().close();
  });
}
