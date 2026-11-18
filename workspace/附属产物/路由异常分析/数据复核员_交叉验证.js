// 数据复核员_交叉验证.js — 用 node 独立复核 python 主脚本的口径与数字。
// 键名用 String.fromCharCode 构造，避免任何字面量在管道中被改写影响判断。
const fs = require("fs");
const PATH = "C:/Users/10215/.config/magpie/usage.jsonl";
// 候选键名 A: o z e m j g q j ; 候选键名 B: p r o v i d e r
const kA = String.fromCharCode(0x6f, 0x7a, 0x65, 0x6d, 0x6a, 0x67, 0x71, 0x6a);
const kB = String.fromCharCode(0x70, 0x72, 0x6f, 0x76, 0x69, 0x64, 0x65, 0x72);
const START = Date.parse("2026-11-12T00:00:00+08:00");

let total = 0, parseFail = 0, noT = 0, outWin = 0, inWin = 0;
let emptyChan = 0, probeEx = 0, counted = 0, failed = 0, rejTrue = 0;
const agents = Object.create(null);
const chans = Object.create(null);
let firstWin = null, lastWin = null;
let keyA = 0, keyB = 0, keyOther = 0;

function chanOf(rec) {
  if (Object.prototype.hasOwnProperty.call(rec, kA)) { keyA++; return rec[kA]; }
  if (Object.prototype.hasOwnProperty.call(rec, kB)) { keyB++; return rec[kB]; }
  keyOther++; return undefined;
}

const lines = fs.readFileSync(PATH, "utf8").split(/\r?\n/);
for (const raw of lines) {
  const s = raw.trim();
  if (!s) continue;
  total++;
  let rec;
  try { rec = JSON.parse(s); } catch { parseFail++; continue; }
  const t = Date.parse(rec.t);
  if (Number.isNaN(t)) { noT++; continue; }
  if (t < START) { outWin++; continue; }
  inWin++;
  if (!firstWin || t < firstWin) firstWin = t;
  if (!lastWin || t > lastWin) lastWin = t;
  const oz = chanOf(rec);
  if (oz === undefined || oz === null || oz === "") {
    emptyChan++;
    if (rec.kind === "model_list" || rec.source === "probe") probeEx++;
    continue;
  }
  if (rec.kind === "model_list" || rec.source === "probe") { probeEx++; continue; }
  counted++;
  const name = String(oz).toLowerCase();
  if (!chans[name]) chans[name] = { req: 0, failed: 0 };
  chans[name].req++;
  const ag = rec.agent === undefined || rec.agent === null ? "(none)" : String(rec.agent);
  if (!agents[ag]) agents[ag] = { req: 0, failed: 0 };
  agents[ag].req++;
  const st = rec.status;
  const stOk = Number.isInteger(st) && st >= 200 && st <= 299;
  const errBad = typeof rec.err === "string" && rec.err.length > 0;
  if (rec.rejected === true) rejTrue++;
  if (!stOk || errBad || rec.rejected === true) {
    failed++;
    chans[name].failed++;
    agents[ag].failed++;
  }
}

console.log("== 键名探测（免疫字面量改写）==");
console.log("keyA(6F 7A 65 6D 6A 67 71 6A) 命中记录数:", keyA);
console.log("keyB(70 72 6F 76 69 64 65 72) 命中记录数:", keyB);
console.log("keyOther:", keyOther);
console.log();
console.log("== 独立重算（node）==");
console.log("lines_total=%d parse_fail=%d no_t=%d out_win=%d in_win=%d", total, parseFail, noT, outWin, inWin);
console.log("empty_chan=%d probe_excluded=%d counted=%d failed=%d rejected_true=%d", emptyChan, probeEx, counted, failed, rejTrue);
console.log("窗口覆盖: %s .. %s", new Date(firstWin).toISOString(), new Date(lastWin).toISOString());
console.log("总体失败率= %.4f%%", (100 * failed / counted));
console.log();
console.log("== agent 复核（zcode / claude-desktop）==");
for (const a of ["zcode", "claude-desktop", "dsh", "other", "curl", "magpie", "claude"]) {
  const r = agents[a];
  console.log("%-16s req=%s failed=%s", a, r ? r.req : 0, r ? r.failed : 0);
}
console.log();
console.log("== 抽查渠道（对照 python 表）==");
for (const c of ["dumate", "workbuddy", "zcode", "groq", "zed", "gemini", "nvidia", "77625", "codar", "qoder"]) {
  const r = chans[c];
  console.log("%-16s req=%s failed=%s", c, r ? r.req : 0, r ? r.failed : 0);
}
console.log();
const chanSum = Object.values(chans).reduce((s, c) => s + c.req, 0);
const failSum = Object.values(chans).reduce((s, c) => s + c.failed, 0);
console.log("一致性: 渠道请求合计=%d (应=counted %d) 渠道失败合计=%d (应=failed %d)", chanSum, counted, failSum, failed);
console.log("渠道个数=%d", Object.keys(chans).length);
