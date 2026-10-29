//go:build darwin

// Command procsample samples the memory and CPU of a desktop client together
// with every process macOS counts against it: Electron helpers are its
// children, WebKit content processes are spawned by launchd but carry the
// app as their responsible process. It is the external half of the
// client comparison in benchmarks/README.md.
//
//	procsample -app Freelens -for 60s -csv out.csv
package main

/*
#include <libproc.h>
#include <sys/resource.h>
#include <mach/mach_time.h>

extern int responsibility_get_pid_responsible_for_pid(pid_t);

static int usage(pid_t pid, uint64_t *footprint, uint64_t *cpu) {
	struct rusage_info_v2 ri;
	if (proc_pid_rusage(pid, RUSAGE_INFO_V2, (rusage_info_t *)&ri) != 0) return -1;
	*footprint = ri.ri_phys_footprint;
	*cpu = ri.ri_user_time + ri.ri_system_time;
	return 0;
}

static double tick_ns(void) {
	mach_timebase_info_data_t tb;
	mach_timebase_info(&tb);
	return (double)tb.numer / tb.denom;
}
*/
import "C"

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type proc struct {
	pid, ppid int
	comm      string
}

func main() {
	app := flag.String("app", "", "substring of the main process path, e.g. Freelens.app/Contents/MacOS")
	pid := flag.Int("pid", 0, "main process id instead of -app")
	dur := flag.Duration("for", 30*time.Second, "how long to sample")
	every := flag.Duration("every", time.Second, "sample interval")
	csvPath := flag.String("csv", "", "write one row per sample here")
	flag.Parse()

	root := *pid
	if root == 0 {
		if *app == "" {
			fail("need -app or -pid")
		}
		root = find(*app)
	}
	tick := float64(C.tick_ns())

	var out *os.File
	if *csvPath != "" {
		f, err := os.Create(*csvPath)
		if err != nil {
			fail(err.Error())
		}
		defer f.Close()
		out = f
		fmt.Fprintln(out, "t_s,procs,footprint_mb,cpu_pct")
	}

	var prevCPU float64
	var peak, sum float64
	var cpuSum float64
	n := 0
	start := time.Now()
	prevT := start
	for time.Since(start) <= *dur {
		group := tree(root)
		if len(group) == 0 {
			fail(fmt.Sprintf("process %d is gone", root))
		}
		var fp, cpu uint64
		for _, p := range group {
			var f, c C.uint64_t
			if C.usage(C.pid_t(p.pid), &f, &c) == 0 {
				fp += uint64(f)
				cpu += uint64(c)
			}
		}
		now := time.Now()
		cpuNs := float64(cpu) * tick
		mb := float64(fp) / (1 << 20)
		pct := 0.0
		if n > 0 {
			pct = (cpuNs - prevCPU) / float64(now.Sub(prevT).Nanoseconds()) * 100
			cpuSum += pct
		}
		prevCPU, prevT = cpuNs, now
		peak = max(peak, mb)
		sum += mb
		n++
		if out != nil {
			fmt.Fprintf(out, "%.1f,%d,%.0f,%.1f\n", now.Sub(start).Seconds(), len(group), mb, pct)
		}
		if n == 1 {
			fmt.Println(describe(group))
		}
		time.Sleep(*every)
	}
	fmt.Printf("memory: last %.0f MB, mean %.0f MB, peak %.0f MB | CPU mean %.1f%% over %d samples\n",
		lastMB(root), sum/float64(n), peak, cpuSum/float64(max(n-1, 1)), n)
}

func lastMB(root int) float64 {
	var total uint64
	for _, p := range tree(root) {
		var f, c C.uint64_t
		if C.usage(C.pid_t(p.pid), &f, &c) == 0 {
			total += uint64(f)
		}
	}
	return float64(total) / (1 << 20)
}

// tree returns root, its descendants and every process whose responsible
// process is one of them.
func tree(root int) []proc {
	all := list()
	in := map[int]bool{root: true}
	for changed := true; changed; {
		changed = false
		for _, p := range all {
			if in[p.pid] {
				continue
			}
			if in[p.ppid] || in[int(C.responsibility_get_pid_responsible_for_pid(C.pid_t(p.pid)))] {
				in[p.pid] = true
				changed = true
			}
		}
	}
	var group []proc
	for _, p := range all {
		if in[p.pid] {
			group = append(group, p)
		}
	}
	return group
}

func list() []proc {
	b, err := exec.Command("ps", "-axo", "pid=,ppid=,comm=").Output()
	if err != nil {
		fail(err.Error())
	}
	var ps []proc
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		ps = append(ps, proc{pid, ppid, strings.Join(f[2:], " ")})
	}
	return ps
}

func find(sub string) int {
	var hits []proc
	for _, p := range list() {
		if strings.Contains(p.comm, sub) && !strings.Contains(p.comm, "Helper") {
			hits = append(hits, p)
		}
	}
	if len(hits) == 0 {
		fail("no process matches " + sub)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].pid < hits[j].pid })
	return hits[0].pid
}

func describe(group []proc) string {
	names := map[string]int{}
	for _, p := range group {
		names[p.comm[strings.LastIndex(p.comm, "/")+1:]]++
	}
	parts := make([]string, 0, len(names))
	for k, v := range names {
		parts = append(parts, fmt.Sprintf("%s×%d", k, v))
	}
	sort.Strings(parts)
	return fmt.Sprintf("pid %d, %d processes: %s", group[0].pid, len(group), strings.Join(parts, ", "))
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "procsample:", msg)
	os.Exit(1)
}
