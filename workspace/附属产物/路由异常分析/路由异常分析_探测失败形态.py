# -*- coding: utf-8 -*-
"""探测失败形态的行：rest、非200尝试、done=false、left列表。流式扫描，每类各打印1个。"""
import gzip, json, sys, io

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
ROUTING_DIR = r"C:/Users/10215/.config/magpie/routing"

def trunc(v, n=200):
    s = json.dumps(v, ensure_ascii=False) if not isinstance(v, str) else v
    if len(s) > n:
        return s[:n] + "...<%d>" % len(s)
    return s

def probe(path, is_gzip, max_lines=None):
    op = gzip.open if is_gzip else open
    found = {"rest": False, "fail": False, "notdone": False}
    print("=" * 16, path.split("/")[-1], "=" * 16)
    n = 0
    with op(path, "rt", encoding="utf-8", errors="replace") as f:
        for line in f:
            n += 1
            if max_lines and n > max_lines:
                break
            try:
                o = json.loads(line)
            except Exception:
                continue
            # rest
            if not found["rest"]:
                for e in (o.get("order") or []) + (o.get("left") or []):
                    if isinstance(e, dict) and e.get("rest"):
                        found["rest"] = True
                        print("--REST sample: id=%s time=%s" % (o.get("id"), o.get("time")))
                        print("   order entry: id=%s provider=%s rest=%s" % (e.get("id"), e.get("provider"), trunc(e.get("rest"))))
                        break
            # failed try
            if not found["fail"]:
                for t in o.get("tries") or []:
                    if isinstance(t, dict) and t.get("status") != 200:
                        found["fail"] = True
                        print("--FAILTRY sample: id=%s time=%s done=%s finalStatus=%s" % (o.get("id"), o.get("time"), o.get("done"), o.get("status")))
                        print("   try: %s" % trunc({k: v for k, v in t.items() if k != "net"}, 700))
                        break
            if not found["notdone"] and o.get("done") is not True:
                found["notdone"] = True
                print("--NOTDONE sample: id=%s time=%s done=%s status=%s" % (o.get("id"), o.get("time"), o.get("done"), o.get("status")))
                print("   tries statuses: %s" % [ (t.get("status"), t.get("done")) for t in (o.get("tries") or [])])
                print("   keys: %s" % sorted(o.keys()))
            if all(found.values()):
                break
    print("scanned %d lines, found: %s" % (n, found))

probe(ROUTING_DIR + "/2026-11-18.jsonl", False)
probe(ROUTING_DIR + "/2026-11-17.jsonl", False)
probe(ROUTING_DIR + "/2026-11-16.jsonl.gz", True)
