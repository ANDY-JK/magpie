# -*- coding: utf-8 -*-
# 检视 usage.jsonl 的结构：键集合、时间戳格式、字段取值分布。
# 只读，不改任何数据。
import json
import re
from collections import Counter
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
    dt = datetime(
        int(m.group(1)), int(m.group(2)), int(m.group(3)),
        int(m.group(4)), int(m.group(5)), int(m.group(6)),
        microsecond=int(frac),
        tzinfo=timezone(sign * timedelta(hours=int(body[:2]), minutes=int(body[2:4]))),
    )
    return dt


lines = 0
bad = 0
keysets = Counter()
key_presence = Counter()
in_window = 0
before_window = 0
t_bad = 0
offsets = Counter()
first_provider_line = None

agents = Counter()
kinds = Counter()
sources = Counter()
rejected_types = Counter()
status_types = Counter()
provider_values = Counter()
err_types = Counter()
window_key_presence = Counter()

samples = []

with open(PATH, "r", encoding="utf-8", errors="replace") as f:
    for i, line in enumerate(f, 1):
        s = line.strip()
        if not s:
            lines += 1
            continue
        lines += 1
        try:
            rec = json.loads(s)
        except Exception:
            bad += 1
            if len(samples) < 3:
                samples.append((i, "PARSE_FAIL", s[:120]))
            continue
        if not isinstance(rec, dict):
            bad += 1
            continue
        keysets[tuple(sorted(rec.keys()))] += 1
        for k in rec:
            key_presence[k] += 1
        t = rec.get("t")
        dt = None
        if isinstance(t, str):
            dt = parse_t(t)
            m = TS_RE.match(t)
            if m:
                offsets[m.group(8) or "(none)"] += 1
        if dt is None:
            t_bad += 1
            continue
        if dt >= WIN_START:
            in_window += 1
            for k in rec:
                window_key_presence[k] += 1
            if rec.get("provider") is not None and first_provider_line is None:
                first_provider_line = i
            a = rec.get("agent")
            agents[a if a is None else str(a)] += 1
            kinds[rec.get("kind", "(missing)")] += 1
            sources[rec.get("source", "(missing)")] += 1
            rejected_types[type(rec.get("rejected")).__name__ + ":" + str(rec.get("rejected"))] += 1
            st = rec.get("status")
            status_types[type(st).__name__ + ":" + str(st)] += 1
            er = rec.get("err")
            err_types[type(er).__name__] += 1
            oz = rec.get("provider")
            provider_values["(missing)" if "provider" not in rec else ("(empty)" if oz == "" else str(oz))] += 1
            if len(samples) < 6 and rec.get("provider"):
                samples.append((i, "IN_WIN_SAMPLE", json.dumps(rec, ensure_ascii=False)[:300]))
        else:
            before_window += 1

print("== 文件规模 ==")
print("total_lines:", lines, "parse_fail:", bad, "t_unparseable:", t_bad)
print("in_window(>=2026-11-12T00:00+08:00):", in_window, "before_window:", before_window)
print()
print("== 时间戳偏移分布 ==")
for k, v in offsets.most_common():
    print("offset", k, ":", v)
print()
print("== 键出现次数（全文件 top）==")
for k, v in key_presence.most_common(30):
    print(k, ":", v)
print()
print("== 窗口内键出现次数 ==")
for k, v in window_key_presence.most_common(40):
    print(k, ":", v)
print()
print("== 窗口内 agent 分布 ==")
for k, v in agents.most_common():
    print(repr(k), ":", v)
print()
print("== 窗口内 kind 分布 ==")
for k, v in kinds.most_common():
    print(repr(k), ":", v)
print()
print("== 窗口内 source 分布 ==")
for k, v in sources.most_common():
    print(repr(k), ":", v)
print()
print("== 窗口内 rejected 类型 ==")
for k, v in rejected_types.most_common():
    print(k, ":", v)
print()
print("== 窗口内 status 类型/取值 ==")
for k, v in status_types.most_common(30):
    print(k, ":", v)
print()
print("== 窗口内 err 值类型 ==")
for k, v in err_types.most_common():
    print(k, ":", v)
print()
print("== 窗口内 provider 取值 ==")
for k, v in provider_values.most_common():
    print(repr(k), ":", v)
print()
print("first_provider_line:", first_provider_line)
print()
print("== 样例 ==")
for i, tag, s in samples:
    print(i, tag, s)
