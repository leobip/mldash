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
func StartNodeRefresher(ctx context.Context, client kubernetes.Interface, store *mlcache.Store) {
	refreshNodes(ctx, client, store)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshNodes(ctx, client, store)
			}
		}
	}()
}

func refreshNodes(ctx context.Context, client kubernetes.Interface, store *mlcache.Store) {
	list, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("node refresh error: %v", err)
		return
	}
	seen := make(map[string]bool)
	for i := range list.Items {
		info := buildNodeInfo(&list.Items[i])
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

func buildNodeInfo(node *corev1.Node) models.NodeInfo {
	gpuQ := node.Status.Allocatable["nvidia.com/gpu"]
	gpuTotal, _ := gpuQ.AsInt64()

	ready := false
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}

	return models.NodeInfo{
		Name:        node.Name,
		Accelerator: node.Labels["accelerator"],
		GPUTotal:    gpuTotal,
		Ready:       ready,
	}
}
