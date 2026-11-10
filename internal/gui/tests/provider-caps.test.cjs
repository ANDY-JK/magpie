// Run with Node's test runner and Playwright on the module path; see README.md.
// The owner (11-10): 测试模型时顺便测试模型能力并列出. A provider's editor has
// Test capabilities beside Test models: one small request more per model asks
// it to call a tool, to read a 1×1 picture and to stream, and each chip then
// says T, V and S — green for the endpoint's yes, red for its no, nothing
// where it said nothing — with the words on hover, and the footer counts them.
// A chip's right-click tests that model's capabilities alone. Test models
// still sends no capability work. English and Chinese, both engines; no
// backend, the API is faked.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const CAPS = {
  m1: { tools: "yes", vision: "no", stream: "yes" },
  m2: { tools: "no", vision: "yes" },
};
const relay = {
  id: "caps", name: "Caps Relay", icon: "generic", chat: "https://caps.example.com/v1", responses: "", anthropic: "", catalog: "",
  models: [
    { id: "m1", name: "m1", on: true, context: 128000, output: 16384, efforts: ["high"] },
    { id: "m2", name: "m2", on: true, context: 32000, output: 8192, images: true },
  ],
  agents: [], fallback: [], headers: {}, key: { set: true, masked: "sk-…caps" }, keyList: [],
  balanceToken: { takes: false, set: false }, proxy: "",
};

function serve(lang, tests) {
  const providers = { providers: [relay], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/provider/test") {
      const body = route.request().postDataJSON();
      tests.push(body);
      return json({ results: body.test.map((id) => ({ protocol: "chat", ok: true, ms: 12, model: id, ...(body.caps ? { caps: CAPS[id] } : {}) })) });
    }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const L = {
  en: {
    test: "Test capabilities", menu: "Test its capabilities", all: "Test models",
    count: "Tools: 1 of 2 · vision: 1 · streaming: 1",
    tools: "Tools: it called the tool it was asked to", noTools: "Tools: it didn't call a tool",
    vision: "Vision: it took an image", noVision: "Vision: it turned an image down",
    stream: "Streaming: it answered as a stream", legend: "T tools · V vision · S streaming — ✓ it answered, ✗ it refused",
    one: "m1 answered in 12 ms · T✓ V✗ S✓", took: "12 ms",
  },
  zh: {
    test: "测试能力", menu: "测试它的能力", all: "测试模型",
    count: "工具：1/2 · 视觉：1 · 流式：1",
    tools: "工具：它按请求调用了工具", noTools: "工具：它没有调用工具",
    vision: "视觉：它接收了图片", noVision: "视觉：它拒绝了图片",
    stream: "流式：它用流式回答", legend: "T 工具 · V 视觉 · S 流式 — ✓ 已应答，✗ 被拒绝",
    one: "m1 在 12 毫秒内响应 · T✓ V✗ S✓", took: "12 毫秒",
  },
};
const KEYS = ["Test capabilities", "Test its capabilities", "Tools: {t} of {n} · vision: {v} · streaming: {s}",
  "Tools: it called the tool it was asked to", "Tools: it didn't call a tool", "Vision: it took an image",
  "Vision: it turned an image down", "Streaming: it answered as a stream", "Streaming: it didn't stream",
  "T tools · V vision · S streaming — ✓ it answered, ✗ it refused"];

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of ["en", "zh"]) {
    const w = L[lang];
    test(`${engine} ${lang}: a model's capabilities are tested and listed`, async (t) => {
      const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
      const page = await (await browser.newContext({ viewport: { width: 980, height: 820 }, reducedMotion: "reduce", locale: lang === "zh" ? "zh-CN" : "en-US" })).newPage();
      t.after(() => browser.close());
      page.setDefaultTimeout(5000);
      page.setDefaultNavigationTimeout(60000);
      const errors = [], tests = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.route("**/*", serve(lang, tests));
      await page.goto("http://magpie.test/?view=providers");
      await page.locator(".row.provider", { hasText: "Caps Relay" }).click();
      const ed = page.locator(".editor");
      await ed.locator(".mchips .mchip").first().waitFor();
      const chip = (id) => ed.locator(".mchips .mchip.pick", { hasText: id });
      const wait = async (n) => { for (let i = 0; i < 80 && tests.length < n; i++) await page.waitForTimeout(50); };
      const badges = async (id) => chip(id).locator(".badge.cap").evaluateAll((bs) => bs.map((b) => ({ text: b.textContent, cls: b.className, title: b.title })));

      // the press asks for capabilities of every model served
      await ed.locator(".mfoot").getByRole("button", { name: w.test, exact: true }).click();
      await wait(1);
      assert.deepEqual({ id: tests[0].id, test: tests[0].test, caps: tests[0].caps }, { id: "caps", test: ["m1", "m2"], caps: true });
      await page.waitForFunction((want) => document.querySelector("#status")?.textContent === want, w.count);
      // T, V and S, green for yes and red for no, the words on hover
      assert.deepEqual(await badges("m1"), [
        { text: "T", cls: "badge cap ok", title: w.tools },
        { text: "V", cls: "badge cap bad", title: w.noVision },
        { text: "S", cls: "badge cap ok", title: w.stream },
      ]);
      assert.deepEqual(await badges("m2"), [
        { text: "T", cls: "badge cap bad", title: w.noTools },
        { text: "V", cls: "badge cap ok", title: w.vision },
      ], "a capability the answer didn't settle has no letter");
      // the legend says what the letters are, and the facts stay in the hover
      assert.equal((await ed.locator(".cap-legend").textContent()).trim(), w.legend);
      assert(await ed.locator(".cap-legend").isVisible());
      assert((await chip("m1").getAttribute("title")).includes(w.took), "the test's answer is in the hover beside the facts");

      // one model's capabilities alone, from its right-click
      const n = tests.length;
      await chip("m2").click({ button: "right" });
      const menu = page.locator(".pop.row-menu");
      await menu.waitFor();
      await menu.getByRole("menuitem", { name: w.menu }).click();
      await wait(n + 1);
      assert.deepEqual({ id: tests.at(-1).id, test: tests.at(-1).test, caps: tests.at(-1).caps }, { id: "caps", test: ["m2"], caps: true });
      await page.waitForTimeout(300);
      assert((await page.locator("#status").textContent()).includes("T✗ V✓"), await page.locator("#status").textContent());

      // Test models still asks only whether they answer
      const m = tests.length;
      await ed.locator(".mfoot").getByRole("button", { name: w.all, exact: true }).click();
      await wait(m + 1);
      assert.equal(tests.at(-1).caps, undefined, "Test models asks no capability");
      const missing = KEYS.filter((k) => !require("./strings.cjs").strings().I18N.zh[k]);
      assert.deepEqual(missing, [], "every new word has its Chinese");
      assert.deepEqual(errors, []);
    });
  }
}
