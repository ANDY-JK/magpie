# -*- coding: utf-8 -*-
"""抽样路由日志，确认行结构与字段名。只打印结构+截断值，不整行输出。"""
import gzip, json, sys, io

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")

ROUTING_DIR = r"C:/Users/10215/.config/magpie/routing"

def trunc(v, n=90):
    s = json.dumps(v, ensure_ascii=False) if not isinstance(v, str) else v
    if len(s) > n:
        return s[:n] + "...<%d chars>" % len(s)
    return s

def show(obj, prefix="", depth=0, maxdepth=4):
    if depth >= maxdepth:
        print(prefix + ": <deeper>")
        return
    if isinstance(obj, dict):
        for k, v in obj.items():
            if isinstance(v, dict):
                print("%s%s: dict(%d)" % (prefix, k, len(v)))
                show(v, prefix + "  " + k + ".", depth + 1, maxdepth)
            elif isinstance(v, list):
                print("%s%s: list(%d)" % (prefix, k, len(v)))
                if v:
                    if isinstance(v[0], dict):
                        show(v[0], prefix + "  " + k + "[0].", depth + 1, maxdepth)
                    else:
                        print("%s  %s[0]= %s" % (prefix, k, trunc(v[0], 60)))
            else:
                print("%s%s = %s" % (prefix, k, trunc(v)))

def sample(path, n_lines, is_gzip):
    op = gzip.open if is_gzip else open
    print("=" * 20, path, "=" * 20)
    with op(path, "rt", encoding="utf-8", errors="replace") as f:
        for i in range(n_lines):
            line = f.readline()
            if not line:
                break
            try:
                obj = json.loads(line)
            except Exception as e:
                print("line %d parse error: %s" % (i + 1, e))
                continue
            print("----- line %d (len=%d bytes) -----" % (i + 1, len(line.encode("utf-8", errors="replace"))))
            show(obj)

sample(ROUTING_DIR + "/2026-11-18.jsonl", 2, False)
sample(ROUTING_DIR + "/2026-11-17.jsonl", 2, False)
sample(ROUTING_DIR + "/2026-11-16.jsonl.gz", 2, True)
