# k9s_bench.py [RUNS] [WANT]: k9s in tmux, timed until its pod count reaches WANT,
# then 30 s of memory and CPU. Needs KUBECONFIG and PROCSAMPLE (bin/procsample).
import os, re, subprocess, sys, time, tempfile
runs = int(sys.argv[1]) if len(sys.argv) > 1 else 5
want = int(sys.argv[2]) if len(sys.argv) > 2 else 50000
env = dict(os.environ)
def sh(*a): return subprocess.run(a, capture_output=True, text=True).stdout
for r in range(runs):
    cfg = tempfile.mkdtemp(prefix="k9scfg")
    sh("tmux", "kill-session", "-t", "k9sb")
    t0 = time.monotonic()
    subprocess.run(["tmux", "new-session", "-d", "-s", "k9sb", "-x", "220", "-y", "60",
        f"env K9S_CONFIG_DIR={cfg} KUBECONFIG={env['KUBECONFIG']} AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null k9s --context kind-kwok -A -c pods --splashless --readonly"], check=True)
    first = full = None
    count = 0
    while time.monotonic() - t0 < 180:
        pane = sh("tmux", "capture-pane", "-p", "-t", "k9sb")
        t = time.monotonic() - t0
        if first is None and "load-" in pane: first = t
        m = re.search(r"pods\(all\)\[([\d,]+)\]", pane, re.I)
        if m: count = int(m.group(1).replace(",", ""))
        if count >= want: full = t; break
        time.sleep(0.02)
    pid = sh("tmux", "list-panes", "-t", "k9sb", "-F", "#{pane_pid}").strip()
    kp = sh("pgrep", "-P", pid, "k9s").strip() or pid
    mem = sh(os.environ["PROCSAMPLE"], "-pid", kp.split()[0], "-for", "30s").strip().splitlines()[-1]
    print(f"k9s {r+1}: first rows {first:.2f}s, full ({count}) {full and f'{full:.2f}s'} | {mem}", flush=True)
    sh("tmux", "kill-session", "-t", "k9sb")
    time.sleep(5)
