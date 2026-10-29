// Command loadgen creates many pods in a test cluster to measure informer
// memory and UI responsiveness.
//
// Pods are pinned to a node label that no node has, so they stay Pending and
// cost the cluster nothing but etcd space. With -kwok they go to the fake
// nodes of stand/kwok-stand.sh instead and reach Running, as in a real
// cluster, still without containers. Never point it at a real cluster:
// it refuses contexts whose name does not start with "kind-".
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeContext := flag.String("context", "kind-kubedrive", "kubeconfig context (must start with kind-)")
	namespace := flag.String("namespace", "kubedrive-load", "namespace for generated pods")
	pods := flag.Int("pods", 2000, "number of pods to create")
	workers := flag.Int("workers", 32, "parallel requests")
	clean := flag.Bool("delete", false, "delete the namespace instead of creating pods")
	kwok := flag.Bool("kwok", false, "schedule the pods on kwok's fake nodes (stand/kwok-stand.sh)")
	flag.Parse()

	if err := run(*kubeContext, *namespace, *pods, *workers, *clean, *kwok); err != nil {
		fmt.Fprintln(os.Stderr, "loadgen:", err)
		os.Exit(1)
	}
}

func run(kubeContext, namespace string, pods, workers int, clean, kwok bool) error {
	if !strings.HasPrefix(kubeContext, "kind-") {
		return fmt.Errorf("refusing to run against %q: only kind-* contexts are allowed", kubeContext)
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		return err
	}
	cfg.QPS, cfg.Burst = 500, 1000
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	ctx := context.Background()

	if clean {
		err := kube.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	_, err = kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	start := time.Now()
	var created, failed atomic.Int64
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				_, err := kube.CoreV1().Pods(namespace).Create(ctx, pod(i, kwok), metav1.CreateOptions{})
				switch {
				case err == nil:
					created.Add(1)
				case apierrors.IsAlreadyExists(err):
				default:
					failed.Add(1)
				}
			}
		}()
	}
	for i := range pods {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	fmt.Printf("created %d pods (%d failed) in %s\n", created.Load(), failed.Load(), time.Since(start).Round(time.Millisecond))
	return nil
}

func pod(i int, kwok bool) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("load-%06d", i),
			Labels: map[string]string{"app.kubernetes.io/part-of": "kubedrive-load", "shard": fmt.Sprint(i % 20)},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"kubedrive.dev/nonexistent": "true"},
			Containers:   []corev1.Container{{Name: "pause", Image: "registry.k8s.io/pause:3.10"}},
		},
	}
	if kwok {
		p.Spec.NodeSelector = map[string]string{"type": "kwok"}
		p.Spec.Tolerations = []corev1.Toleration{{Key: "kwok.x-k8s.io/node", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	}
	return p
}
