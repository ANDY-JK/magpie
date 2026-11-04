// Run with Node's test runner and Playwright on the module path; see README.md.
// Balanced spends what comes back soonest and keeps a paid plan's windows for
// last: a request first goes to a day's quota, points or credits, then to a
// plan's five hours, its week and its month, and to a wallet's money last
// (provider.Allowance.Class). The Routing page's story says so for the seat it
// picked, and says why when one window is about to renew with much of it left,
// which goes before them all. Seats of one class are told by their pace as
// before. In every language, wide and narrow.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const now = new Date();
const H = 3600e3;
const ahead = (hours) => new Date(now.getTime() + hours * H).toISOString();
const ago = (minutes) => new Date(now.getTime() - minutes * 60e3).toISOString();

// a seat as weighRouted reads it, its class taken from what it spends
const seat = (id, cls, fields) => ({
  provider: "opencode-go", name: "OpenCode Go", kind: "account", agent: "opencode", model: "glm-5",
  known: true, used: 20, pace: 0.5, due: ahead(72), renews: [ahead(72)], routing: "balance", class: cls, ...fields,
});
const day = seat("opencode-go#day", 0, { who: "day@example.com", used: 30 });
const five = seat("opencode-go#five", 1, { who: "five@example.com", used: 10 });
const month = seat("opencode-go#month", 3, { who: "month@example.com" });
const month2 = seat("opencode-go#month2", 3, { who: "month2@example.com" });
// a month in the last tenth of its span with a fifth of it unspent
const losing = seat("opencode-go#losing", 3, { who: "losing@example.com", used: 15, loses: true });

const route = (id, order) => ({
  id, seq: id, time: ago(1), agent: "opencode", model: "glm-5", provider: "opencode-go", done: true, status: 200, ms: 900, tokens: 1200,
  order, tries: [{ id: order[0].id, model: "glm-5", start: ago(1), done: true, status: 200, ms: 900 }],
});
const routes = [
  route(1, [day, five, month]), // the day's quota before the plan's windows
  route(2, [losing, day]), // about to be lost first, whatever it spends
  route(3, [month, month2]), // alike: told by pace, as before
];

function serve(lang) {
  return async (request) => {
    const url = new URL(request.request().url());
    const json = (data) => request.fulfill({ json: data });
    if (url.pathname === "/boot.js") return request.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return request.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json({ agents: [{ id: "opencode", name: "OpenCode Go", path: "/test/config.toml", fields: [] }], profiles: [], settings: { lang, theme: "light" } });
    if (url.pathname === "/api/gateway/trace") {
      if (url.searchParams.get("wait")) return new Promise(() => {}); // nothing more comes
      return json({ mine: true, now: now.toISOString(), seq: 3, totals: { requests: routes.length }, routes });
    }
    if (url.pathname === "/api/gateway/history") return json({ cut: false, days: [], routes: [] });
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/providers") return json({ providers: [], excluded: [], gateway: { running: true, window: true } });
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await request.fulfill({ body: await fs.readFile(file), contentType });
  };
}

// sooner: what it has comes back sooner than the others'; lost: one window
// about to renew with much of it unspent; pace: alike classes, told as before
const words = {
  en: {
    sooner: /Balanced: day@example\.com goes first: what it has comes back sooner than the others' quota, and theirs is kept for later\./,
    lost: /Balanced: losing@example\.com goes first: one of its windows renews soon with much of it unspent/,
    pace: /Balanced: month@example\.com has the most of its allowance left per hour until it is lost/,
  },
  zh: {
    sooner: /均衡：day@example\.com 先走：它的额度比别人的更快恢复/,
    lost: /均衡：losing@example\.com 先走：它的某个窗口很快重置/,
    pace: /均衡：month@example\.com 的剩余额度 ÷ 距作废小时数最高/,
  },
  "zh-TW": {
    sooner: /均衡：day@example\.com 先走：它的額度比別人的更快恢復/,
    lost: /均衡：losing@example\.com 先走：它的某個視窗很快重置/,
    pace: /均衡：month@example\.com 的剩餘額度 ÷ 距作廢小時數最高/,
  },
  ja: {
    sooner: /バランス：day@example\.com が先です。その枠はほかより早く戻る/,
    lost: /バランス：losing@example\.com が先です。枠のひとつがまもなくリセットされる/,
    pace: /バランス：month@example\.com は期限までの 1 時間あたりの残り利用枠が最も多い/,
  },
  de: {
    sooner: /Ausgeglichen: day@example\.com zuerst: sein Kontingent kommt schneller zurück/,
    lost: /Ausgeglichen: losing@example\.com zuerst: eines seiner Fenster wird bald zurückgesetzt/,
    pace: /Ausgeglichen: month@example\.com hat das meiste Restkontingent pro Stunde/,
  },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of Object.keys(words)) {
    for (const width of [1100, 420]) {
      test(`${engine} ${lang} ${width}px: Balanced tells the quota that comes back soonest, and one about to be lost`, async (t) => {
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        t.after(() => browser.close());
        const page = await browser.newPage({ viewport: { width, height: 1200 }, reducedMotion: "reduce" });
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        const errors = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.route("**/*", serve(lang));
        await page.goto("http://magpie.test/?view=routing");
        await page.locator(".rt-req").nth(routes.length - 1).waitFor();
        const w = words[lang];
        const why = () => page.locator(".rt-steps li.why").first().textContent();
        const open = async (n, who, want) => {
          await page.locator(".rt-req").nth(n).click();
          await page.waitForFunction(([who, src]) => {
            const li = document.querySelector(".rt-steps li.why");
            return li && li.textContent.includes(who) && new RegExp(src).test(li.textContent);
          }, [who, want.source]);
        };
        // the page lists the newest request first: route 3, 2, 1
        // one about to be lost goes before the rest, whatever it spends
        await open(1, "losing@example.com", w.lost);
        assert.doesNotMatch(await why(), w.sooner);
        // the day's quota before the plan's windows, and said so
        await open(2, "day@example.com", w.sooner);
        assert.doesNotMatch(await why(), w.lost);
        // alike classes: told by their pace, as before
        await open(0, "month@example.com", w.pace);
        assert.doesNotMatch(await why(), w.sooner);
        assert.doesNotMatch(await why(), w.lost);
        const over = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
        assert.ok(over <= 0, `the page scrolls sideways by ${over}px`);
        assert.deepEqual(errors, []);
      });
    }
  }
}
