package watchers

import (
	"context"
	"log"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	mlcache "github.com/leobip/mldash/internal/cache"
)

// StartPodNodeRefresher lists pods in Kubeflow user namespaces periodically
// to build a pod→nodeName map used to resolve GPU types.
// nsPrefix selects namespaces by prefix (e.g. "kubeflow-").
func StartPodNodeRefresher(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, nsPrefix string) {
	refreshPodNodes(ctx, client, store, nsPrefix)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshPodNodes(ctx, client, store, nsPrefix)
			}
		}
	}()
}

func refreshPodNodes(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, nsPrefix string) {
	nsList, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("podnodes: namespace list error: %v", err)
		return
	}
	for _, ns := range nsList.Items {
		if !strings.HasPrefix(ns.Name, nsPrefix) {
			continue
		}
		pods, err := client.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("podnodes: pod list error in %s: %v", ns.Name, err)
			continue
		}
		for _, pod := range pods.Items {
			if pod.Spec.NodeName != "" {
				store.SetPodNode(pod.Namespace, pod.Name, pod.Spec.NodeName)
			}
		}
	}
}
