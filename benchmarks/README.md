# KubeDrive benchmarks

How fast Kubernetes desktop clients open a large list of pods, measured from outside each
client the same way. The results are on [kubedrive.com/performance](https://kubedrive.com/performance).
Everything here is MIT-licensed (see [LICENSE](LICENSE)), so you can rerun it on your own
machine and clusters.

## What is measured

Each client starts cold with a kubeconfig that holds only the test cluster and opens pods in all
namespaces. For every run:

- **Window**: from launch to the client's first window.
- **First rows**: from launch to the first frame that shows a pod row (`load-000123`).
- **Full list**: until the client's own object count shows every pod. Clients without a count
  (Headlamp) are not timed here; k9s and kubectl print the whole list at once.
- **Memory and idle CPU**: 30 s with the list open, over the whole process tree: Electron helpers,
  WebKit content processes and other processes macOS counts against the app.

Median of five runs (Freelens on 50,000 pods: three). A run starts when the cluster's control
plane uses less than 40% CPU, since its VM shares the Mac's processor.

## Layout

| Path | What it is |
|---|---|
| `stand/kwok-stand.sh` | a kind cluster with [kwok](https://kwok.sigs.k8s.io) fake nodes, so pods reach Running without containers |
| `stand/loadgen` | creates the pods (`load-000000`…); refuses contexts not named `kind-*` |
| `probes/screenprobe` | launches a client, records only its windows (ScreenCaptureKit), reads frames with Vision OCR, makes the clicks a client needs |
| `probes/procsample` | memory (phys_footprint, as Activity Monitor) and CPU of a process tree |
| `run/run.sh` | five cold runs of a desktop client |
| `run/k9s_bench.py`, `run/kubectl_bench.py` | the same for k9s (in tmux) and `kubectl get pods -A` |
| `results/` | raw output of every run, by date |

## Running it

Needs macOS 13+, Xcode command line tools, Go, Docker (we used Colima, 4 CPU, 8 GB), kind,
kubectl, and Screen Recording and Accessibility permissions for your terminal.

```sh
xcrun swiftc -O probes/screenprobe/main.swift -o bin/screenprobe
go build -o bin/procsample ./probes/procsample

export KUBECONFIG=$PWD/kwok.yaml
stand/kwok-stand.sh 470 110                 # or 4 12500 for the small cluster
go run ./stand/loadgen -context kind-kwok -kwok -pods 50000

mkdir -p home/headlamp/.kube && cp $KUBECONFIG home/headlamp/.kube/config
run/run.sh headlamp 5
PROCSAMPLE=bin/procsample python3 run/k9s_bench.py 5 50010
python3 run/kubectl_bench.py 5
```

`run/run.sh` clicks at screen points measured on a 1680×1050 display; on another screen, adjust
the `--then` points.

## Results of 2026-10-29

A 2018 MacBook Pro (Intel Core i7, 16 GB). Versions: KubeDrive build of 2026-10-29, Aptakube
1.22.1, Headlamp 0.46.0, Freelens 2.0.0, k9s 0.51.0.

**50,000 pods on 470 nodes**

| Client | Window | First rows | Full list | Memory | Idle CPU |
|---|---|---|---|---|---|
| KubeDrive | 1.37 s | 2.98 s | 10.4 s | 428 MB | 1.6% |
| Headlamp | 1.31 s | 6.4 s | no count | 590 MB | 5.2% |
| Freelens | 5.4 s | none in 150 s (3 of 3) | — | 530 MB | 3.0% |
| k9s | — | 26.4 s | 26.4 s | 2.6 GB | 276% |
| `kubectl get pods -A` | — | 25.5 s | 27.0 s | — | — |

Aptakube's free plan opens clusters of up to 5 nodes, so it is not in this run.

**50,000 pods on 4 nodes**

| Client | Window | First rows | Full list | Memory | Idle CPU |
|---|---|---|---|---|---|
| KubeDrive | 0.62 s | 1.53 s | 5.3 s | 444 MB | 1.3% |
| Headlamp | 0.81 s | 4.6 s | no count | 707 MB | 3.3% |
| Aptakube | 0.52 s | 16.9 s | 16.9 s | 880 MB | 0.0% |
| Freelens | 2.2 s | none in 150 s (2 of 3); 38.8 s | — | 500–1,367 MB | 1–9% |
| k9s | — | 34 s | 34 s | 2.6 GB | 305% |
| `kubectl get pods -A` | — | 24.6 s | 26.0 s | — | — |

**500 pods on 4 nodes**

| Client | Window | First rows | Memory | Idle CPU |
|---|---|---|---|---|
| KubeDrive | 0.59 s | 1.49 s | 291 MB | 1.4% |
| k9s | — | 1.80 s | 82 MB | 4.0% |
| Aptakube | 0.45 s | 2.32 s | 467 MB | 0.0% |
| Headlamp | 0.74 s | 3.92 s | 349 MB | 2.3% |
| Freelens | 1.93 s | 6.07 s | 685 MB | 3.8% |
| `kubectl get pods -A` | — | 0.19 s | — | — |
