# -*- coding: utf-8 -*-
"""深挖失败链：最深链、全断簇构成、换道救援的深度分布、挑完整案例。"""
import json, sys, io, os
from collections import Counter

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8")
D = os.path.dirname(os.path.abspath(__file__))
chains = json.load(open(os.path.join(D, "路由异常分析_失败链.json"), encoding="utf-8"))
res = json.load(open(os.path.join(D, "路由异常分析_结果.json"), encoding="utf-8"))

print("total failed chains:", len(chains))

# 1) 最深全败链
by_depth = sorted(chains, key=lambda c: -len(c["tries"]))
print("\n== top-5 deepest failed chains ==")
for c in by_depth[:5]:
    seq = " -> ".join("%s(%s%s)" % (t["provider"], t["status"], "/" + (t["fail"] or "") if t["fail"] else "") for t in c["tries"])
    print("id=%s time=%s agent=%s model=%s req_status=%s" % (c["id"], c["time"][:19], c["agent"], c["model"], c["req_status"]))
    print("   seq[%d]: %s" % (len(c["tries"]), seq[:900]))
    print("   errs:", [ (t["provider"], t["status"], (t["error"] or "")[:80]) for t in c["tries"][:4] ], "...")

# 2) 大簇 11-17 17:11 ~ 11-18 00:02 构成
big = [c for c in chains if "2026-11-17T17:11" <= c["time"][:16] <= "2026-11-18T00:03"]
print("\n== big cluster 11-17 17:11 ~ 11-18 00:02, n=%d ==" % len(big))
print("agent:", Counter(c["agent"] for c in big).most_common(5))
print("model:", Counter(c["model"] for c in big).most_common(6))
print("first-try provider:", Counter(c["tries"][0]["provider"] if c["tries"] else "none" for c in big).most_common(8))
print("req_status:", Counter(str(c["req_status"]) for c in big).most_common())
print("lasthop:", Counter("%s/%s" % (c["tries"][-1]["provider"], c["tries"][-1]["status"]) if c["tries"] else "no-try" for c in big).most_common(8))
print("depth:", Counter(len(c["tries"]) for c in big).most_common())

# 3) 另一个大簇 11-16 20:47 窗口构成
w2 = [c for c in chains if "2026-11-16T20:47" <= c["time"][:16] <= "2026-11-16T21:09"]
print("\n== cluster 11-16 20:47 ~ 21:09, n=%d ==" % len(w2))
print("model:", Counter(c["model"] for c in w2).most_common(5))
print("first provider:", Counter(c["tries"][0]["provider"] if c["tries"] else "none" for c in w2).most_common(6))
print("lasthop:", Counter("%s/%s/%s" % (c["tries"][-1]["provider"], c["tries"][-1]["status"], (c["tries"][-1]["fail"] or "")) if c["tries"] else "no-try" for c in w2).most_common(6))
print("sample errs:", [ (t["error"] or "")[:110] for c in w2[:3] for t in c["tries"][-1:] ])

# 4) 499 canceled 的失败链（客户端取消）：agent/model 分布
c99 = [c for c in chains if str(c["req_status"]) == "499"]
print("\n== 499 canceled failed requests: n=%d ==" % len(c99))
print("agent:", Counter(c["agent"] for c in c99).most_common(6))
print("lasthop fail:", Counter(c["tries"][-1]["fail"] if c["tries"] else "none" for c in c99).most_common(4))
print("lasthop error sample:", [ (c["tries"][-1]["error"] or "")[:100] for c in c99[:5] ])

# 5) 候选案例：跨 3+ 渠道全败、有 quota/rate/credit 混合、时间完整
cands = []
for c in chains:
    if len(c["tries"]) >= 3:
        provs = [t["provider"] for t in c["tries"]]
        cands.append(c)
print("\n== candidate case chains (depth>=3): n=%d ==" % len(cands))
for c in cands[:10]:
    seq = " -> ".join("%s:%s%s" % (t["provider"], t["status"], "@" + (t["model"] or "")[:24] if t["model"] else "") for t in c["tries"])
    print("id=%s t=%s agent=%s model=%s\n   %s\n   last_err=%s" % (c["id"], c["time"][:19], c["agent"], c["model"], seq[:500], (c["tries"][-1]["error"] or "")[:140]))

# 6) 深度34那条的完整链
deep34 = [c for c in chains if len(c["tries"]) == 34]
for c in deep34:
    print("\n== depth-34 chain id=%s t=%s agent=%s model=%s ==" % (c["id"], c["time"][:19], c["agent"], c["model"]))
    for t in c["tries"]:
        print("  try%02d %s %-18s st=%-4s ms=%-7s ttft=%-7s fail=%-8s err=%s" % (t["i"], (t["start"] or "")[11:19], t["provider"], t["status"], t["ms"], t["ttft"], t["fail"], (t["error"] or "")[:90]))
    print("  rest candidates:", json.dumps(c["rest"], ensure_ascii=False))
