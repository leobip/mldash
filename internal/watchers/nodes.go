package watchers

import (
	"context"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	mlcache "github.com/leobip/mldash/internal/cache"
	"github.com/leobip/mldash/internal/models"
)

// StartNodeRefresher does an initial node list and refreshes every 30s.
// Uses List only (no Watch) — compatible with get/list permissions on nodes.
func StartNodeRefresher(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, gpuResourceKeys []string, gpuTypeLabel string) {
	refreshNodes(ctx, client, store, gpuResourceKeys, gpuTypeLabel)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshNodes(ctx, client, store, gpuResourceKeys, gpuTypeLabel)
			}
		}
	}()
}

func refreshNodes(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, gpuResourceKeys []string, gpuTypeLabel string) {
	list, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("node refresh error: %v", err)
		return
	}
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase!=Succeeded,status.phase!=Failed",
	})
	if err != nil {
		log.Printf("node refresh: pod list error: %v", err)
		return
	}
	usage := sumPodRequests(pods.Items, gpuResourceKeys)
	seen := make(map[string]bool)
	for i := range list.Items {
		info := buildNodeInfo(&list.Items[i], usage[list.Items[i].Name], gpuResourceKeys, gpuTypeLabel)
		store.SetNode(info)
		seen[list.Items[i].Name] = true
	}
	// remove nodes that disappeared
	for _, n := range store.GetNodes() {
		if !seen[n.Name] {
			store.DeleteNode(n.Name)
		}
	}
	log.Printf("nodes refreshed: %d total", len(list.Items))
}

func buildNodeInfo(node *corev1.Node, used nodeUsage, gpuResourceKeys []string, gpuTypeLabel string) models.NodeInfo {
	gpuTotal := SumGPU(node.Status.Allocatable, gpuResourceKeys)

	ready := false
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	cpuTotal := node.Status.Allocatable.Cpu().MilliValue()
	memTotal := node.Status.Allocatable.Memory().Value()

	return models.NodeInfo{
		Name:        node.Name,
		Accelerator: node.Labels[gpuTypeLabel],
		Ready:       ready,

		GPUTotal:     gpuTotal,
		GPUUsed:      used.gpu,
		GPUAvailable: gpuTotal - used.gpu,

		CPUTotalMilli:     cpuTotal,
		CPUUsedMilli:      used.cpuMilli,
		CPUAvailableMilli: cpuTotal - used.cpuMilli,

		MemTotalBytes:     memTotal,
		MemUsedBytes:      used.memBytes,
		MemAvailableBytes: memTotal - used.memBytes,
	}
}

// nodeUsage is the sum of resources requested by pods scheduled on a node.
type nodeUsage struct {
	gpu      int64
	cpuMilli int64
	memBytes int64
}

// sumPodRequests adds up container requests per node name. Pods not yet
// scheduled (empty nodeName) are skipped. initContainers are not counted
// (the scheduler uses max(init, regular); this is an approximation).
func sumPodRequests(pods []corev1.Pod, gpuResourceKeys []string) map[string]nodeUsage {
	usage := make(map[string]nodeUsage)
	for i := range pods {
		pod := &pods[i]
		if pod.Spec.NodeName == "" {
			continue
		}
		u := usage[pod.Spec.NodeName]
		for _, c := range pod.Spec.Containers {
			u.gpu += SumGPU(c.Resources.Requests, gpuResourceKeys)
			u.cpuMilli += c.Resources.Requests.Cpu().MilliValue()
			u.memBytes += c.Resources.Requests.Memory().Value()
		}
		usage[pod.Spec.NodeName] = u
	}
	return usage
}
