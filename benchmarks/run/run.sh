#!/bin/zsh
# run.sh CLIENT [RUNS] [DONE_RE]: cold launches of a desktop client, then 30 s of
# memory and CPU with the pods list open. One line per run on stdout.
#
#   TMO=150 run/run.sh headlamp 5
#   run/run.sh kubedrive 5 '50[ ,.]?0[01][0-9] (items|Pods)'
#
# Each client runs with its own HOME (home/<client>, with .kube/config holding
# only the stand's context), so it sees no other cluster and keeps no state
# from your own use. The --when/--then clicks are screen points of a
# 1680x1050 display with the window where the client opens it; on another
# screen, find them with a screenshot (points = pixels / backing scale).
# Build the probes first: xcrun swiftc -O probes/screenprobe/main.swift -o
# bin/screenprobe; go build -o bin/procsample ./probes/procsample
B=${0:A:h:h}
c=$1; runs=${2:-5}; done_re=$3
H=$B/home/$c
envs=(--env HOME=$H --env KUBECONFIG=$H/.kube/config --env AWS_CONFIG_FILE=/dev/null --env AWS_SHARED_CREDENTIALS_FILE=/dev/null)
case $c in
  kubedrive) app=/Applications/KubeDrive.app; steps=() ;;
  freelens)  app=/Applications/Freelens.app
             steps=(--when 'Welcome to Freelens' --then 157,226 --when 'Cron Jobs' --then 250,299) ;;
  headlamp)  app=/Applications/Headlamp.app
             steps=(--when 'All Clusters' --then 364,438 --when 'Workloads' --then 87,413 --when 'Leader Worker' --then 67,338) ;;
  aptakube)  app=/Applications/Aptakube.app
             steps=(--when 'CronJobs' --then 36,403) ;;
  *) echo "usage: $0 kubedrive|freelens|headlamp|aptakube [runs] [done-regexp]" >&2; exit 2 ;;
esac
id=$(defaults read $app/Contents/Info.plist CFBundleIdentifier)
done_args=(); [[ -n $done_re ]] && done_args=(--done "$done_re")
for r in $(seq 1 $runs); do
  osascript -e "tell application id \"$id\" to quit" 2>/dev/null
  for i in {1..40}; do pgrep -f "$app/Contents/MacOS" >/dev/null || break; sleep 0.25; done
  sleep 4
  # The stand's VM shares the Mac's CPU: start only when its control plane idles.
  for i in {1..60}; do
    cpu=$(docker stats --no-stream --format '{{.CPUPerc}}' kwok-control-plane | tr -d '%')
    (( ${cpu%.*} < 40 )) && break
    sleep 5
  done
  line=$($B/bin/screenprobe launch $app $envs $steps --match 'load-\d{6}' $done_args --timeout ${TMO:-150} 2>>$B/err.log)
  pid=$(echo $line | sed -E 's/.*pid=([0-9]+).*/\1/')
  sleep 5
  mem=$($B/bin/procsample -pid $pid -for 30s | tail -1)
  echo "$c $r: $line | $mem | stand cpu before ${cpu}%"
done
osascript -e "tell application id \"$id\" to quit" 2>/dev/null
