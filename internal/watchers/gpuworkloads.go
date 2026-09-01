package watchers

import (
	"context"
	"log"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	mlcache "github.com/leobip/mldash/internal/cache"
	"github.com/leobip/mldash/internal/models"
)

// StartGPUWorkloadRefresher lists GPU-requesting pods cluster-wide, skipping
// namespaces that belong to Kubeflow notebooks (kubeflowNSPrefix) since those
// are already tracked via the Notebook CRD informer.
func StartGPUWorkloadRefresher(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, kubeflowNSPrefix string, gpuResourceKeys []string, gpuTypeLabel string) {
	refreshGPUWorkloads(ctx, client, store, kubeflowNSPrefix, gpuResourceKeys, gpuTypeLabel)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshGPUWorkloads(ctx, client, store, kubeflowNSPrefix, gpuResourceKeys, gpuTypeLabel)
			}
		}
	}()
}

func refreshGPUWorkloads(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, kubeflowNSPrefix string, gpuResourceKeys []string, gpuTypeLabel string) {
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("gpuworkloads: pod list error: %v", err)
		return
	}

	seen := map[string]bool{}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if strings.HasPrefix(pod.Namespace, kubeflowNSPrefix) {
			continue // already tracked via the Notebook CRD informer
		}
		w, ok := buildGPUWorkload(pod, gpuResourceKeys, gpuTypeLabel)
		if !ok {
			continue
		}
		store.SetWorkload(w)
		seen[pod.Namespace+"/"+pod.Name] = true
	}

	// remove disappeared workloads
	for _, w := range store.GetWorkloads() {
		if !seen[w.Namespace+"/"+w.Name] {
			store.DeleteWorkload(w.Namespace, w.Name)
		}
	}
}

func buildGPUWorkload(pod *corev1.Pod, gpuResourceKeys []string, gpuTypeLabel string) (models.GPUWorkload, bool) {
	var gpuCount int64
	var cpuReq, memReq, image string

	for _, c := range pod.Spec.Containers {
		n := SumGPU(c.Resources.Requests, gpuResourceKeys)
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
	gpuType := pod.Spec.NodeSelector[gpuTypeLabel]
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
