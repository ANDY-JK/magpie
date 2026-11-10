// The GUI tests that run fastest and cover the ground this season's work
// touched: the provider editor's tabs and fetched models, Allowances' tabs,
// Routing's tabs, the Library's lists and export, the image MCP's words, the
// balance help, Vertex's editors and the doctor. They are a smoke run, not
// the whole suite (`node --test internal/gui/tests/` is that), and every one
// here passed on Chromium and WebKit when it was added.
//
//   node scripts/gui-smoke.cjs            both engines
//   BROWSER=chromium node scripts/gui-smoke.cjs
const { spawnSync } = require("node:child_process");
const path = require("node:path");

const files = [
  "balance-help-short.test.cjs",
  "doctor.test.cjs",
  "image-mcp.test.cjs",
  "library-export.test.cjs",
  "library-list-filter.test.cjs",
  "library-subagents-bulk.test.cjs",
  "models-fold.test.cjs",
  "provider-editor-tabs.test.cjs",
  "provider-fetch-again.test.cjs",
  "routing-tabs.test.cjs",
  "settings-allowance-tabs.test.cjs",
  "vertex-editor.test.cjs",
];

const dir = path.join(__dirname, "..", "internal", "gui", "tests");
const args = ["--test", "--test-concurrency=1", ...files.map((f) => path.join(dir, f))];
const r = spawnSync(process.execPath, args, { stdio: "inherit", env: process.env });
if (r.error) {
  console.error(r.error);
  process.exit(1);
}
process.exit(r.status ?? 1);
