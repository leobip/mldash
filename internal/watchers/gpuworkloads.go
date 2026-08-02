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

// StartGPUWorkloadRefresher lists pods with GPU requests in the given namespaces.
// If namespaces is empty, the refresher does nothing.
func StartGPUWorkloadRefresher(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, namespaces []string) {
	if len(namespaces) == 0 {
		return
	}
	refreshGPUWorkloads(ctx, client, store, namespaces)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshGPUWorkloads(ctx, client, store, namespaces)
			}
		}
	}()
}

func refreshGPUWorkloads(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, namespaces []string) {
	for _, ns := range namespaces {
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("gpuworkloads: pod list error in %s: %v", ns, err)
			continue
		}
		seen := map[string]bool{}
		for i := range pods.Items {
			pod := &pods.Items[i]
			w, ok := buildGPUWorkload(pod)
			if !ok {
				continue
			}
			store.SetWorkload(w)
			seen[ns+"/"+pod.Name] = true
		}
		// remove disappeared workloads in this namespace
		for _, w := range store.GetWorkloads() {
			if w.Namespace == ns && !seen[ns+"/"+w.Name] {
				store.DeleteWorkload(ns, w.Name)
			}
		}
	}
}

func buildGPUWorkload(pod *corev1.Pod) (models.GPUWorkload, bool) {
	var gpuCount int64
	var cpuReq, memReq, image string

	for _, c := range pod.Spec.Containers {
		gpuQ := c.Resources.Requests["nvidia.com/gpu"]
		n, _ := gpuQ.AsInt64()
		gpuCount += n
		if n > 0 {
			image = c.Image
			if q := c.Resources.Requests[corev1.ResourceCPU]; !q.IsZero() {
				cpuReq = q.String()
			}
			if q := c.Resources.Requests[corev1.ResourceMemory]; !q.IsZero() {
				memReq = q.String()
			}
		}
	}

	if gpuCount == 0 {
		return models.GPUWorkload{}, false
	}

	// GPU type: from nodeSelector if Pending, from nodeName lookup later
	gpuType := pod.Spec.NodeSelector["accelerator"]
	status := string(pod.Status.Phase)

	return models.GPUWorkload{
		Name:       pod.Name,
		Namespace:  pod.Namespace,
		Image:      image,
		GPUCount:   gpuCount,
		GPUType:    gpuType,
		CPURequest: cpuReq,
		MemRequest: memReq,
		Status:     status,
		NodeName:   pod.Spec.NodeName,
	}, true
}
