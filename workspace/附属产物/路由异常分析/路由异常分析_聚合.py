# -*- coding: utf-8 -*-
"""magpie 路由日志聚合分析：失败链全景 / 换道救援 / 渠道稳定性 / 截断 / 全断 / 小时爆发。
流式逐行处理；11-16 用 gzip 流，不落盘。输出 JSON 到本目录。"""
import gzip, json, sys, io, os, re
from datetime import datetime, timezone, timedelta

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
ROUTING_DIR = r"C:/Users/10215/.config/magpie/routing"
OUT_DIR = os.path.dirname(os.path.abspath(__file__))
TZ = timezone(timedelta(hours=8))

FILES = [
    ("2026-11-16", ROUTING_DIR + "/2026-11-16.jsonl.gz", True),
    ("2026-11-17", ROUTING_DIR + "/2026-11-17.jsonl.gz", True),  # 11-18 00:43 已被轮转为 gz
    ("2026-11-18", ROUTING_DIR + "/2026-11-18.jsonl", False),
]

def trunc(s, n):
    if s is None:
        return None
    s = str(s)
    return s if len(s) <= n else s[:n] + "…"

def epoch_ms(t):
    if not t:
        return None
    m = re.match(r"^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\.?(\d+)?([+-]\d{2}:\d{2})?$", t)
    if not m:
        return None
    frac = (m.group(2) or "")[:6].ljust(6, "0")
    off = m.group(3) or "+08:00"
    try:
        dt = datetime.fromisoformat(m.group(1) + "." + frac + off)
    except ValueError:
        return None
    return dt.timestamp() * 1000.0

def hour_key(t):
    return t[:13] if t else None  # 2026-11-17T00

def pct(sorted_vals, q):
    if not sorted_vals:
        return None
    i = min(len(sorted_vals) - 1, max(0, int(round((len(sorted_vals) - 1) * q))))
    return sorted_vals[i]

# ---- 聚合容器 ----
req_total = 0
req_by_day = {}
req_inflight = 0                       # done 字段缺失
req_no200 = 0                          # tries 中无任何 200
req_no200_notried = 0                  # 且 tries 为空（全候选 rest）
req_no200_final_status = {}            # 最终 status 分布
req_lasthop = {}                       # 最后一跳 (status,fail,error短) -> n
req_done_false = 0                     # 顶层 done != true

first_try_fail = 0                     # 首跳失败
rescue_diff_id = 0                     # 首跳失败→后续不同 id 200
rescue_same_id = 0                     # 首跳失败→同 id 200
still_all_fail = 0                     # 首跳失败→无任何 200
depth_hist = {}                        # len(tries) 直方（全部请求）
depth_hist_fail = {}                   # 失败请求的深度直方

try_total = 0
try_by_provider = {}                   # provider -> {n, ok, ms[], ttft[], done_false, status_dist}
try_status_dist = {}
try_done_false_list = []               # 样例
try_fail_reason = {}                   # fail 字段分布
try_ttft_abandon = []                  # 非200 且 ttft>=60s 样例
try_rest_quota = {}                    # try级 rest.why=quota -> provider 计数

cand_rest = {}                         # provider -> {why: n}
cand_rest_quota = {}                   # provider -> n（order+left 条目级）
req_usage_bad = []                     # status200 但 usage 异常样例
req_usage_bad_n = 0

fail_chain_times = []                  # (epoch, id)
fail_chains = []                       # 失败请求案例链（全量、轻量）
hourly_fail = {}                       # (provider, hour) -> n
hourly_fail_req = {}                   # hour -> 失败请求数
hourly_req = {}                        # hour -> 总请求数

def prov_of(tid, idmap):
    p = idmap.get(tid)
    if p:
        return p
    for sep in ("#", "@"):
        if sep in tid:
            return tid.split(sep)[0]
    return tid or "?"

def err_short(t):
    return trunc((t.get("fail") or "") + "|" + (t.get("error") or ""), 120)

for day, path, is_gz in FILES:
    op = gzip.open if is_gz else open
    nlines = 0
    bad_lines = 0
    with op(path, "rt", encoding="utf-8", errors="replace") as f:
        for line in f:
            nlines += 1
            try:
                o = json.loads(line)
            except Exception:
                bad_lines += 1
                continue
            req_total += 1
            req_by_day[day] = req_by_day.get(day, 0) + 1
            rid = o.get("id")
            t0 = o.get("time") or ""
            hk = hour_key(t0)
            hourly_req[hk] = hourly_req.get(hk, 0) + 1
            if "done" not in o:
                req_inflight += 1
            elif o.get("done") is not True:
                req_done_false += 1

            # 候选表：id -> provider
            idmap = {}
            for e in (o.get("order") or []) + (o.get("left") or []):
                if isinstance(e, dict) and e.get("id"):
                    idmap[e["id"]] = e.get("provider") or prov_of(e["id"], {})
                    r = e.get("rest")
                    if r:
                        p = e.get("provider") or prov_of(e["id"], {})
                        d = cand_rest.setdefault(p, {})
                        d[r.get("why") or "?"] = d.get(r.get("why") or "?", 0) + 1
                        if r.get("why") == "quota":
                            cand_rest_quota[p] = cand_rest_quota.get(p, 0) + 1

            tries = [t for t in (o.get("tries") or []) if isinstance(t, dict)]
            depth = len(tries)
            depth_hist[depth] = depth_hist.get(depth, 0) + 1
            has200 = any(t.get("status") == 200 for t in tries)

            # try 级
            for t in tries:
                try_total += 1
                p = prov_of(t.get("id") or "", idmap)
                d = try_by_provider.setdefault(p, {"n": 0, "ok": 0, "ms": [], "ttft": [], "done_false": 0, "quota_rest": 0})
                d["n"] += 1
                st = t.get("status")
                if st == 200:
                    d["ok"] += 1
                    if t.get("ms") is not None:
                        d["ms"].append(t["ms"])
                    if t.get("ttft") is not None:
                        d["ttft"].append(t["ttft"])
                else:
                    try_status_dist[(p, st)] = try_status_dist.get((p, st), 0) + 1
                    fr = t.get("fail") or "?"
                    try_fail_reason[fr] = try_fail_reason.get(fr, 0) + 1
                    if t.get("done") is not True:
                        d["done_false"] += 1
                        if len(try_done_false_list) < 20:
                            try_done_false_list.append({
                                "req": rid, "time": t0, "provider": p,
                                "try": {k: t.get(k) for k in ("start", "status", "ms", "ttft", "fail", "error")},
                                "req_done": o.get("done"), "req_status": o.get("status")})
                    if t.get("ttft") is not None and t["ttft"] >= 60000 and len(try_ttft_abandon) < 20:
                        try_ttft_abandon.append({
                            "req": rid, "time": t0, "provider": p,
                            "try": {k: t.get(k) for k in ("start", "status", "ms", "ttft", "done", "fail", "error")}})
                    r = t.get("rest")
                    if r and r.get("why") == "quota":
                        d["quota_rest"] += 1
                        try_rest_quota[p] = try_rest_quota.get(p, 0) + 1
                    if st is not None:
                        hourly_fail[(p, hk)] = hourly_fail.get((p, hk), 0) + 1

            # 请求级
            if not has200:
                req_no200 += 1
                hourly_fail_req[hk] = hourly_fail_req.get(hk, 0) + 1
                if depth == 0:
                    req_no200_notried += 1
                fs = o.get("status")
                req_no200_final_status[fs] = req_no200_final_status.get(fs, 0) + 1
                if tries:
                    lt = tries[-1]
                    lk = (lt.get("status"), lt.get("fail") or "?", trunc(lt.get("error") or "", 90))
                    req_lasthop[lk] = req_lasthop.get(lk, 0) + 1
                else:
                    req_lasthop[("no-try", "all-rest", "")] = req_lasthop.get(("no-try", "all-rest", ""), 0) + 1
                depth_hist_fail[depth] = depth_hist_fail.get(depth, 0) + 1
                ems = epoch_ms(t0)
                fail_chain_times.append((ems, rid))
                if len(fail_chains) < 1500:
                    fail_chains.append({
                        "id": rid, "time": t0, "agent": o.get("agent"), "session": trunc(o.get("session"), 60),
                        "model": o.get("model"), "req_status": o.get("status"), "req_done": o.get("done"),
                        "out": o.get("out"),
                        "usage": [{"provider": u.get("provider"), "out": u.get("out")} for u in (o.get("usage") or [])],
                        "rest": [{"provider": e.get("provider"), "why": (e.get("rest") or {}).get("why"),
                                  "status": (e.get("rest") or {}).get("status")} for e in (o.get("order") or []) + (o.get("left") or []) if isinstance(e, dict) and e.get("rest")],
                        "tries": [{"i": i, "id": t.get("id"), "provider": prov_of(t.get("id") or "", idmap), "model": t.get("model"),
                                   "start": t.get("start"), "status": t.get("status"), "done": t.get("done"),
                                   "ms": t.get("ms"), "ttft": t.get("ttft"), "fail": t.get("fail"),
                                   "error": trunc(t.get("error"), 140)} for i, t in enumerate(tries)]})
            # 200 但 usage 异常
            if o.get("status") == 200 and o.get("done") is True:
                usage = o.get("usage") or []
                out_top = o.get("out")
                bad = None
                if not usage:
                    bad = "usage-missing"
                else:
                    outs = [u.get("out") for u in usage]
                    if all(x in (0, None) for x in outs):
                        bad = "out-zero"
                if bad:
                    req_usage_bad_n += 1
                    if len(req_usage_bad) < 15:
                        req_usage_bad.append({"id": rid, "time": t0, "agent": o.get("agent"), "model": o.get("model"),
                                              "kind": bad, "out": out_top,
                                              "usage": [{"provider": u.get("provider"), "out": u.get("out"), "in": u.get("in")} for u in usage],
                                              "n_tries": depth})
            # 换道
            if tries and tries[0].get("status") != 200:
                first_try_fail += 1
                if has200:
                    ok_ids = [t.get("id") for t in tries if t.get("status") == 200]
                    if ok_ids and ok_ids[0] != tries[0].get("id"):
                        rescue_diff_id += 1
                    else:
                        rescue_same_id += 1
                else:
                    still_all_fail += 1

    print("day %s: %d lines, bad=%d" % (day, nlines, bad_lines), flush=True)

# ---- 派生统计 ----
def prov_stat():
    out = {}
    for p, d in try_by_provider.items():
        ms = sorted(d["ms"]); tt = sorted(d["ttft"])
        out[p] = {"attempts": d["n"], "ok": d["ok"], "fail": d["n"] - d["ok"],
                  "ok_rate": round(d["ok"] / d["n"], 4) if d["n"] else None,
                  "ms_p50": pct(ms, .5), "ms_p95": pct(ms, .95),
                  "ttft_p50": pct(tt, .5), "ttft_p95": pct(tt, .95),
                  "ttft_n": len(tt), "done_false": d["done_false"], "quota_rest_tries": d["quota_rest"]}
    return out

provider_stats = prov_stat()
top_errors_by_provider = {}
for (p, st), n in try_status_dist.items():
    top_errors_by_provider.setdefault(p, []).append((st, n))
for p in top_errors_by_provider:
    top_errors_by_provider[p].sort(key=lambda x: -x[1])

hourly_fail_series = {}
for (p, h), n in hourly_fail.items():
    hourly_fail_series.setdefault(h, {})[p] = n

# 全断簇：失败请求按时间聚类，间隔<=10min
fail_chain_times = [x for x in fail_chain_times if x[0]]
fail_chain_times.sort()
clusters = []
for ems, rid in fail_chain_times:
    if clusters and ems - clusters[-1]["end_ms"] <= 600000:
        c = clusters[-1]
        c["end_ms"] = max(c["end_ms"], ems)
        c["n"] += 1
        c["ids"].append(rid)
    else:
        clusters.append({"start_ms": ems, "end_ms": ems, "n": 1, "ids": [rid]})
def ms2str(ms):
    return datetime.fromtimestamp(ms / 1000, TZ).strftime("%m-%d %H:%M:%S")
big_clusters = [{"start": ms2str(c["start_ms"]), "end": ms2str(c["end_ms"]),
                 "span_min": round((c["end_ms"] - c["start_ms"]) / 60000, 1), "n": c["n"]}
                for c in clusters if c["n"] >= 3]
big_clusters.sort(key=lambda x: -x["n"])

# top 爆发小时（按渠道）
top_burst = sorted(((n, p, h) for (p, h), n in hourly_fail.items()), reverse=True)[:12]

result = {
    "files": {day: {"lines": n} for day, n in [(d, req_by_day.get(d, 0)) for d, _, _ in FILES]},
    "requests": {
        "total": req_total, "inflight_no_done": req_inflight, "top_done_false": req_done_false,
        "no200": req_no200, "no200_share": round(req_no200 / req_total, 4) if req_total else None,
        "no200_notried": req_no200_notried,
        "no200_final_status": {str(k): v for k, v in sorted(req_no200_final_status.items(), key=lambda x: -x[1])},
        "first_try_fail": first_try_fail,
        "rescue_diff_provider_id": rescue_diff_id, "rescue_same_id": rescue_same_id,
        "still_all_fail_after_first_fail": still_all_fail,
        "depth_hist_all": {str(k): v for k, v in sorted(depth_hist.items())},
        "depth_hist_failed": {str(k): v for k, v in sorted(depth_hist_fail.items())},
        "usage_bad_200": req_usage_bad_n,
    },
    "last_hop_errors_failed": [{"status": k[0], "fail": k[1], "error": k[2], "n": v}
                               for k, v in sorted(req_lasthop.items(), key=lambda x: -x[1])[:12]],
    "providers": provider_stats,
    "top_status_by_provider": {p: [{"status": s, "n": n} for s, n in v[:6]] for p, v in top_errors_by_provider.items()},
    "try_fail_reasons": dict(sorted(try_fail_reason.items(), key=lambda x: -x[1])),
    "candidate_rest_by_provider": {p: dict(v) for p, v in sorted(cand_rest.items())},
    "candidate_rest_quota_by_provider": dict(sorted(cand_rest_quota.items(), key=lambda x: -x[1])),
    "try_done_false_samples": try_done_false_list,
    "ttft_abandon_samples": try_ttft_abandon,
    "usage_bad_samples": req_usage_bad,
    "all_down_clusters_n3": big_clusters[:15],
    "hourly_fail_requests": {h: n for h, n in sorted(hourly_fail_req.items())},
    "hourly_total_requests": {h: n for h, n in sorted(hourly_req.items())},
    "top_provider_hour_failures": [{"n": n, "provider": p, "hour": h} for n, p, h in top_burst],
    "try_total": try_total,
}
with open(os.path.join(OUT_DIR, "路由异常分析_结果.json"), "w", encoding="utf-8") as f:
    json.dump(result, f, ensure_ascii=False, indent=1)
with open(os.path.join(OUT_DIR, "路由异常分析_失败链.json"), "w", encoding="utf-8") as f:
    json.dump(fail_chains, f, ensure_ascii=False, indent=1)
print("requests=%d tries=%d no200=%d (%.2f%%) chains_saved=%d" %
      (req_total, try_total, req_no200, 100.0 * req_no200 / req_total, len(fail_chains)))
print("saved 路由异常分析_结果.json / 路由异常分析_失败链.json")
