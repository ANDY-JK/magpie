# -*- coding: utf-8 -*-
# 独立重算本周 magpie 网关各渠道失败统计（对照口径由任务给定）。
# 口径：窗口 t>=2026-11-12T00:00:00+08:00；渠道字段非空；排除 kind=="model_list" 或 source=="probe"；
#       失败 = status 非 2xx 或 err 非空 或 rejected==true。
# 只读数据文件，不改任何数据。所有数字以本脚本 stdout 为准。
import json
import re
from collections import Counter, defaultdict
from datetime import datetime, timezone, timedelta

PATH = "C:/Users/10215/.config/magpie/usage.jsonl"
WIN_START = datetime(2026, 11, 12, 0, 0, 0, tzinfo=timezone(timedelta(hours=8)))

TS_RE = re.compile(
    r"^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:?\d{2})?$"
)


def parse_t(s):
    m = TS_RE.match(s)
    if not m:
        return None
    frac = ((m.group(7) or "")[:6]).ljust(6, "0")
    off = m.group(8)
    if off in (None, "", "Z"):
        off = "+00:00"
    sign = 1 if off[0] == "+" else -1
    body = off[1:].replace(":", "")
    return datetime(
        int(m.group(1)), int(m.group(2)), int(m.group(3)),
        int(m.group(4)), int(m.group(5)), int(m.group(6)),
        microsecond=int(frac),
        tzinfo=timezone(sign * timedelta(hours=int(body[:2]), minutes=int(body[2:4]))),
    )


def sanitize(s, cap=100):
    # 错误短语只做展示级脱敏：防长 token / sk- 形态的疑似密钥进结论。
    s = str(s).replace("\r", " ").replace("\n", " ")
    s = re.sub(r"sk-[A-Za-z0-9_\-]{6,}", "<key>", s)
    s = re.sub(r"(?i)bearer\s+\S+", "Bearer <tok>", s)
    s = re.sub(r"[A-Za-z0-9_\-.]{28,}", "<tok>", s)
    s = s.strip()
    return s[:cap]


lines_total = 0
blank = 0
parse_fail = 0
no_t = 0
out_of_window = 0
in_window = 0
empty_channel = 0
probe_excluded = 0
probe_with_empty_channel = 0
counted = 0
failed_total = 0
sig_status = 0
sig_err = 0
sig_rej = 0
status0_cnt = 0
err_empty_str_cnt = 0
key_conflict = 0
key_used = Counter()
status_values = Counter()
chan = defaultdict(lambda: {"requests": 0, "failed": 0, "labels": Counter()})
agents = defaultdict(lambda: {"requests": 0, "failed": 0, "labels": Counter()})
global_labels = Counter()
min_t = None
max_t = None
samples = {"zcode": [], "claude-desktop": []}

with open(PATH, "r", encoding="utf-8", errors="replace") as f:
    for i, line in enumerate(f, 1):
        s = line.strip()
        lines_total += 1
        if not s:
            blank += 1
            continue
        try:
            rec = json.loads(s)
        except Exception:
            parse_fail += 1
            continue
        t = rec.get("t")
        dt = parse_t(t) if isinstance(t, str) else None
        if dt is None:
            no_t += 1
            continue
        if dt < WIN_START:
            out_of_window += 1
            continue
        in_window += 1
        if min_t is None or dt < min_t:
            min_t = dt
        if max_t is None or dt > max_t:
            max_t = dt

        # 渠道字段：优先 provider 键；若该键不存在则退回 provider 键（两种命名都兼容）。
        if "provider" in rec:
            oz = rec.get("provider")
            key_used["provider"] += 1
            if "provider" in rec and rec.get("provider") not in (None, "", oz):
                key_conflict += 1
        elif "provider" in rec:
            oz = rec.get("provider")
            key_used["provider"] += 1
        else:
            oz = None
            key_used["(none)"] += 1

        if oz is None or (isinstance(oz, str) and oz == ""):
            empty_channel += 1
            if rec.get("kind") == "model_list" or rec.get("source") == "probe":
                probe_with_empty_channel += 1
            continue
        if rec.get("kind") == "model_list" or rec.get("source") == "probe":
            probe_excluded += 1
            continue

        counted += 1
        name = str(oz).lower()
        c = chan[name]
        c["requests"] += 1
        ag = rec.get("agent")
        ag = "(none)" if ag is None else str(ag)
        a = agents[ag]
        a["requests"] += 1

        st = rec.get("status")
        status_values[str(st)] += 1
        err = rec.get("err")
        err_bad = isinstance(err, str) and len(err) > 0
        if isinstance(err, str) and len(err) == 0:
            err_empty_str_cnt += 1
        rej = rec.get("rejected") is True
        st_ok = isinstance(st, int) and 200 <= st <= 299
        failed = (not st_ok) or err_bad or rej

        if failed:
            failed_total += 1
            c["failed"] += 1
            a["failed"] += 1
            if not st_ok:
                sig_status += 1
                if st == 0:
                    status0_cnt += 1
            if err_bad:
                sig_err += 1
            if rej:
                sig_rej += 1
            if err_bad:
                label = sanitize(err)
            elif rej:
                label = "rejected=true"
            else:
                label = "HTTP %s" % (st,)
            c["labels"][label] += 1
            a["labels"][label] += 1
            global_labels[label] += 1
            if ag in samples and len(samples[ag]) < 2:
                rid = rec.get("rid") or rec.get("route_id")
                samples[ag].append(
                    "line=%d t=%s chan=%s status=%s err=%s rid=%s"
                    % (i, str(t)[:26], name, st, sanitize(err, 60), rid)
                )

# ---- 一致性断言 ----
sum_req = sum(c["requests"] for c in chan.values())
sum_fail = sum(c["failed"] for c in chan.values())
assert sum_req == counted, "channel requests sum %d != counted %d" % (sum_req, counted)
assert sum_fail == failed_total, "channel failed sum %d != failed_total %d" % (sum_fail, failed_total)
assert in_window == empty_channel + probe_excluded + counted, "stage accounting broken"
consistency = "PASS"

# ---- 输出 ----
print("== 阶段流水 ==")
print("lines_total=%d blank=%d parse_fail=%d no_t=%d" % (lines_total, blank, parse_fail, no_t))
print("out_of_window(<%s)=%d" % (WIN_START.isoformat(), out_of_window))
print("in_window=%d  empty_channel=%d  probe_excluded(nonempty-chan)=%d  counted=%d"
      % (in_window, empty_channel, probe_excluded, counted))
print("probe_records_with_empty_channel(不在 counted 流水内)=%d" % probe_with_empty_channel)
print("channel_key_used=%s conflicts=%d" % (dict(key_used), key_conflict))
print("window_cover: %s .. %s (+08:00)" % (min_t.isoformat(), max_t.isoformat()))
print()
print("== 失败信号分解（一条记录可命中多个信号）==")
print("failed_total=%d  by_status_non2xx=%d  by_err_nonempty=%d  by_rejected_true=%d"
      % (failed_total, sig_status, sig_err, sig_rej))
print("status=0 计为失败的条数=%d ; err 为空字符串的条数=%d" % (status0_cnt, err_empty_str_cnt))
print("总体失败率 = %d/%d = %.4f%%" % (failed_total, counted, 100.0 * failed_total / counted))
print("敏感性参考：剔除 status=0 后 = %d/%d = %.4f%%"
      % (failed_total - status0_cnt, counted - status0_cnt,
         100.0 * (failed_total - status0_cnt) / (counted - status0_cnt)))
print("status 取值分布(counted 内)：%s" % dict(status_values.most_common()))
print()
print("== 全局 top 错误短语（脱敏后）==")
for k, v in global_labels.most_common(10):
    print("  %6d  %s" % (v, k))
print()
print("== 渠道表（按 requests 降序）==")
print("%-24s %8s %8s %8s  %s" % ("provider", "req", "failed", "fail%", "topErrors(top3)"))
for name, c in sorted(chan.items(), key=lambda kv: -kv[1]["requests"]):
    rate = 100.0 * c["failed"] / c["requests"] if c["requests"] else 0.0
    tops = "; ".join("%s(%d)" % (k, v) for k, v in c["labels"].most_common(3)) or "-"
    print("%-24s %8d %8d %7.2f%%  %s" % (name, c["requests"], c["failed"], rate, tops))
print()
print("== agent 表（口径同上，含全部 agent）==")
for name, a in sorted(agents.items(), key=lambda kv: -kv[1]["requests"]):
    rate = 100.0 * a["failed"] / a["requests"] if a["requests"] else 0.0
    tops = "; ".join("%s(%d)" % (k, v) for k, v in a["labels"].most_common(3)) or "-"
    print("%-16s req=%6d failed=%6d fail%%=%6.2f%%  top: %s" % (name, a["requests"], a["failed"], rate, tops))
print()
print("== 样例失败记录（zcode / claude-desktop）==")
for ag in ("zcode", "claude-desktop"):
    for smp in samples[ag]:
        print("  [%s] %s" % (ag, smp))
print()
print("== consistency: %s ==" % consistency)

# ---- 最终 JSON（数字与上文一致）----
report = {
    "headline": None,  # 由运行者依据上方数字填写，不在脚本内拼接
    "stats": [
        {
            "provider": name,
            "requests": c["requests"],
            "failed": c["failed"],
            "topErrors": [k for k, _ in c["labels"].most_common(3)],
        }
        for name, c in sorted(chan.items(), key=lambda kv: -kv[1]["requests"])
    ],
}
print()
print("== REPORT_JSON_BEGIN ==")
print(json.dumps(report["stats"], ensure_ascii=False))
print("== REPORT_JSON_END ==")
