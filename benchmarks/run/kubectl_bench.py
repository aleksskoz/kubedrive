# kubectl_bench.py [RUNS]: kubectl get pods -A, first line and all lines.
import subprocess, sys, time
for r in range(int(sys.argv[1]) if len(sys.argv) > 1 else 5):
    t0 = time.monotonic(); first = None; n = 0
    p = subprocess.Popen(["kubectl", "--context", "kind-kwok", "get", "pods", "-A", "--no-headers"], stdout=subprocess.PIPE, text=True)
    for _ in p.stdout:
        n += 1
        if first is None: first = time.monotonic() - t0
    p.wait()
    print(f"kubectl {r+1}: first line {first:.2f}s, all {n} lines {time.monotonic()-t0:.2f}s", flush=True)
    time.sleep(5)
