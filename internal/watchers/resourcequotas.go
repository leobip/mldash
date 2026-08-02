package watchers

import (
	"context"
	"log"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	mlcache "github.com/leobip/mldash/internal/cache"
	"github.com/leobip/mldash/internal/models"
)

// StartResourceQuotaRefresher periodically checks ResourceQuota status for every
// namespace that currently has notebooks, plus any extra namespaces configured
// (e.g. GPU workload namespaces). Namespaces are re-discovered on every tick since
// notebook namespaces can appear/disappear over time.
func StartResourceQuotaRefresher(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, extraNamespaces []string) {
	refreshResourceQuotas(ctx, client, store, extraNamespaces)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshResourceQuotas(ctx, client, store, extraNamespaces)
			}
		}
	}()
}

func refreshResourceQuotas(ctx context.Context, client kubernetes.Interface, store *mlcache.Store, extraNamespaces []string) {
	namespaces := dedupeNamespaces(store.NotebookNamespaces(), extraNamespaces)

	for _, ns := range namespaces {
		list, err := client.CoreV1().ResourceQuotas(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsForbidden(err) {
				store.SetQuota(models.NamespaceQuota{Namespace: ns, Configured: false, Error: "forbidden"})
			} else {
				log.Printf("resourcequotas: list error in %s: %v", ns, err)
			}
			continue
		}
		if len(list.Items) == 0 {
			store.SetQuota(models.NamespaceQuota{Namespace: ns, Configured: false})
			continue
		}
		store.SetQuota(buildNamespaceQuota(ns, list.Items))
	}
}

func dedupeNamespaces(lists ...[]string) []string {
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, l := range lists {
		for _, ns := range l {
			if ns != "" && !seen[ns] {
				seen[ns] = true
				result = append(result, ns)
			}
		}
	}
	return result
}

// buildNamespaceQuota sums CPU/memory hard limits across all ResourceQuota
// objects in the namespace, preferring "requests.*" keys and falling back to
// the plain "cpu"/"memory" keys. Used/Available are computed later in the
// handler layer from live notebook data, not from rq.Status.Used.
func buildNamespaceQuota(ns string, quotas []corev1.ResourceQuota) models.NamespaceQuota {
	var hardCPU, hardMem resource.Quantity

	for _, rq := range quotas {
		addQuantity(&hardCPU, rq.Status.Hard, "requests.cpu", "cpu")
		addQuantity(&hardMem, rq.Status.Hard, "requests.memory", "memory")
	}

	return models.NamespaceQuota{
		Namespace:  ns,
		Configured: true,
		CPUTotal:   hardCPU.String(),
		MemTotal:   hardMem.String(),
	}
}

// addQuantity adds the first matching resource key found in list to acc.
func addQuantity(acc *resource.Quantity, list corev1.ResourceList, keys ...string) {
	for _, k := range keys {
		if q, ok := list[corev1.ResourceName(k)]; ok {
			acc.Add(q)
			return
		}
	}
}
