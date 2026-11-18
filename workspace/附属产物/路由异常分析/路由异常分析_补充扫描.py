# -*- coding: utf-8 -*-
"""补充扫描：换道救援深度直方、各渠道 top 错误短语、agnes 风暴会话集中度。"""
import gzip, json, sys, io, os
from collections import Counter

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8")
ROUTING_DIR = r"C:/Users/10215/.config/magpie/routing"
D = os.path.dirname(os.path.abspath(__file__))
FILES = [
    ("2026-11-16", ROUTING_DIR + "/2026-11-16.jsonl.gz", True),
    ("2026-11-17", ROUTING_DIR + "/2026-11-17.jsonl.gz", True),
    ("2026-11-18", ROUTING_DIR + "/2026-11-18.jsonl", False),
]

rescue_depth = Counter()          # 首跳失败后第 k 跳成功（k 从 2 计）
rescue_day = Counter()
prov_top_err = {}                 # provider -> Counter(error[:70])
agnes_storm_sessions = Counter()  # 11-17 17:11~11-18 00:03 且 model 含 agnes
storm_req_status = Counter()

def prov_of(tid, idmap):
    p = idmap.get(tid)
    if p:
        return p
    for sep in ("#", "@"):
        if sep in tid:
            return tid.split(sep)[0]
    return tid or "?"

for day, path, is_gz in FILES:
    op = gzip.open if is_gz else open
    with op(path, "rt", encoding="utf-8", errors="replace") as f:
        for line in f:
            try:
                o = json.loads(line)
            except Exception:
                continue
            tries = [t for t in (o.get("tries") or []) if isinstance(t, dict)]
            if not tries or tries[0].get("status") == 200:
                continue
            idmap = {}
            for e in (o.get("order") or []) + (o.get("left") or []):
                if isinstance(e, dict) and e.get("id"):
                    idmap[e["id"]] = e.get("provider") or prov_of(e["id"], {})
            ok_at = None
            for i, t in enumerate(tries):
                if t.get("status") == 200:
                    ok_at = i + 1
                    break
            if ok_at:
                rescue_depth[ok_at] += 1
                rescue_day[day] += 1
            for t in tries:
                if t.get("status") != 200:
                    p = prov_of(t.get("id") or "", idmap)
                    msg = (t.get("error") or t.get("fail") or "?")
                    # 归一：status+fail+错误头 24 字（供应商错误尾串常带 request id / 时间，须截头归并）
                    prov_top_err.setdefault(p, Counter())["%s|%s|%s" % (t.get("status"), t.get("fail") or "?", msg[:24])] += 1
            t0 = o.get("time") or ""
            if "2026-11-17T17:11" <= t0[:16] <= "2026-11-18T00:03" and "agnes" in (o.get("model") or ""):
                agnes_storm_sessions[o.get("session")] += 1
                storm_req_status[str(o.get("status"))] += 1

out = {
    "rescue_depth_hist": {str(k): v for k, v in sorted(rescue_depth.items())},
    "rescue_by_day": dict(rescue_day),
    "prov_top_errors": {p: [{"err": e[:70], "n": n} for e, n in c.most_common(3)] for p, c in prov_top_err.items()},
    "agnes_storm_sessions": agnes_storm_sessions.most_common(5),
    "agnes_storm_req_status": dict(storm_req_status),
}
with open(os.path.join(D, "路由异常分析_补充.json"), "w", encoding="utf-8") as f:
    json.dump(out, f, ensure_ascii=False, indent=1)
print(json.dumps(out["rescue_depth_hist"], ensure_ascii=False), "rescued_by_day:", dict(rescue_day))
print("agnes storm sessions:", agnes_storm_sessions.most_common(5), "statuses:", dict(storm_req_status))
for p in ("sophnet", "agnes", "amd", "77625", "nvidia", "qoder-cn", "gemini", "groq", "zcode", "minimax-code", "qwenwork", "workbuddy", "raccoon", "fox-kkqwq", "sensenova-token-plan", "nova-ai", "autoclaw-cn", "codar", "lightvela-plus", "sta1n"):
    print(p, "->", json.dumps(prov_top_err.get(p, {}).most_common(3), ensure_ascii=False))
