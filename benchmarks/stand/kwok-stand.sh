#!/bin/bash
# A benchmark stand: a kind cluster "kwok" (context kind-kwok) with the kwok
# controller, which plays the kubelet for fake nodes, so thousands of pods
# reach Running without containers (benchmarks/README.md).
#   stand/kwok-stand.sh [nodes] [pods-per-node]   create or reshape the stand
#   stand/kwok-stand.sh --delete                  delete the cluster
# Large clusters: 470 nodes of 110 pods. Small ones: 4 nodes of 12500 pods,
# five with the control plane, which keeps Aptakube within its free tier.
# Reshaping deletes fake nodes that are surplus or of another size; their pods
# go with them (pod GC), so refill with loadgen afterwards.
# KUBECONFIG names the file kind writes the context to; pods go there with
#   go run ./stand/loadgen -context kind-kwok -kwok -pods 10000
set -euo pipefail
name=kwok
kwok_version=v0.8.0
nodes=${1:-50}
per_node=${2:-110}

if [[ $nodes == --delete ]]; then
  kind delete cluster --name "$name"
  exit 0
fi
[[ $nodes =~ ^[0-9]+$ && $per_node =~ ^[0-9]+$ ]] ||
  { echo "usage: $0 [nodes] [pods-per-node] | --delete" >&2; exit 2; }

if ! kind get clusters | grep -qx "$name"; then
  # More requests in flight than the default 400/200: the stand is for
  # measuring clients, so the API server should not be what limits them.
  # kubeadm v1beta4 (Kubernetes 1.31+) takes extraArgs as a list.
  kind create cluster --name "$name" --config /dev/stdin <<'KIND'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: ClusterConfiguration
        apiServer:
          extraArgs:
            - { name: max-requests-inflight, value: "1600" }
            - { name: max-mutating-requests-inflight, value: "800" }
        controllerManager:
          extraArgs:
            - { name: kube-api-qps, value: "500" }
            - { name: kube-api-burst, value: "1000" }
        scheduler:
          extraArgs:
            - { name: kube-api-qps, value: "500" }
            - { name: kube-api-burst, value: "1000" }
KIND
fi
k() { kubectl --context "kind-$name" "$@"; }

base=https://github.com/kubernetes-sigs/kwok/releases/download/$kwok_version
k apply -f "$base/kwok.yaml"
k apply -f "$base/stage-fast.yaml"
k -n kube-system rollout status deploy/kwok-controller --timeout=3m

# Capacity is node status, which apply does not change on an existing node,
# so nodes of another size are deleted and created again.
k get nodes -l type=kwok --no-headers \
  -o custom-columns=NAME:.metadata.name,PODS:.status.capacity.pods |
  while read -r node pods; do
    i=${node##*-}
    if ((i >= nodes)) || [[ $pods != "$per_node" ]]; then echo "$node"; fi
  done | xargs -r kubectl --context "kind-$name" delete node --wait=false >/dev/null

# Fake nodes: 110 pods each by default, like a real node; the taint keeps
# real pods (none here but the system's) off them.
for i in $(seq 0 $((nodes - 1))); do
  cat <<NODE
---
apiVersion: v1
kind: Node
metadata:
  name: kwok-node-$i
  annotations: { kwok.x-k8s.io/node: fake }
  labels: { type: kwok, kubernetes.io/role: agent, kubernetes.io/hostname: kwok-node-$i }
spec:
  taints: [{ key: kwok.x-k8s.io/node, value: fake, effect: NoSchedule }]
status:
  allocatable: { cpu: "32", memory: 256Gi, pods: "$per_node" }
  capacity: { cpu: "32", memory: 256Gi, pods: "$per_node" }
NODE
done | k apply -f - >/dev/null
k wait --for=condition=Ready node -l type=kwok --timeout=2m >/dev/null
echo "kind-$name: $(k get nodes -l type=kwok --no-headers | wc -l | tr -d ' ') fake nodes ready"
