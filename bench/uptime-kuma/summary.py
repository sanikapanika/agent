import csv, sys, statistics as st
def mib(v):
    v = v.split("/")[0].strip()
    for unit, f in (("GiB", 1024), ("MiB", 1), ("KiB", 1/1024), ("B", 1/1024/1024)):
        if v.endswith(unit): return float(v[:-len(unit)]) * f
rows = {}
for name, mem, cpu in csv.reader(open(sys.argv[1])):
    rows.setdefault(name, []).append((mib(mem), float(cpu.rstrip("%"))))
for name in ("bench-agent", "bench-kuma", "bench-kuma-slim"):
    r = rows.get(name, [])
    if not r: continue
    m = [x for x, _ in r]; c = [y for _, y in r]
    print(f"{name:16} n={len(r):3}  mem avg {st.mean(m):7.1f} MiB  max {max(m):7.1f}  |  cpu avg {st.mean(c):5.2f}%  max {max(c):6.2f}%")
