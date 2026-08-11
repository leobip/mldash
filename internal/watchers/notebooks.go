package watchers

import (
	"log"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	mlcache "github.com/leobip/mldash/internal/cache"
	"github.com/leobip/mldash/internal/models"
)

var notebookGVR = schema.GroupVersionResource{
	Group:    "kubeflow.org",
	Version:  "v1",
	Resource: "notebooks",
}

// StartNotebookWatcher watches Kubeflow Notebook CRDs cluster-wide.
func StartNotebookWatcher(factory dynamicinformer.DynamicSharedInformerFactory, store *mlcache.Store, gpuResourceKeys []string, gpuTypeLabel string) {
	informer := factory.ForResource(notebookGVR).Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			store.SetNotebook(buildNotebookInfo(u, gpuResourceKeys, gpuTypeLabel))
		},
		UpdateFunc: func(_, newObj interface{}) {
			u, ok := newObj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			store.SetNotebook(buildNotebookInfo(u, gpuResourceKeys, gpuTypeLabel))
		},
		DeleteFunc: func(obj interface{}) {
			u, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			store.DeleteNotebook(u.GetNamespace(), u.GetName())
			log.Printf("notebook deleted: %s/%s", u.GetNamespace(), u.GetName())
		},
	})
}

func buildNotebookInfo(u *unstructured.Unstructured, gpuResourceKeys []string, gpuTypeLabel string) models.NotebookInfo {
	user := u.GetAnnotations()["notebooks.kubeflow.org/creator"]

	readyReplicas, _, _ := unstructured.NestedInt64(u.Object, "status", "readyReplicas")
	status := "Stopped"
	if readyReplicas > 0 {
		status = "Running"
	}

	var gpuCount int64
	var cpuReq, memReq string

	containers, _, _ := unstructured.NestedSlice(u.Object, "spec", "template", "spec", "containers")
	if len(containers) > 0 {
		if c, ok := containers[0].(map[string]interface{}); ok {
			for _, key := range gpuResourceKeys {
				gpuStr, _, _ := unstructured.NestedString(c, "resources", "limits", key)
				if gpuStr != "" {
					n, _ := strconv.ParseInt(gpuStr, 10, 64)
					gpuCount += n
				}
			}
			cpuReq, _, _ = unstructured.NestedString(c, "resources", "requests", "cpu")
			memReq, _, _ = unstructured.NestedString(c, "resources", "requests", "memory")
		}
	}

	return models.NotebookInfo{
		Name:       u.GetName(),
		Namespace:  u.GetNamespace(),
		User:       user,
		GPUCount:   gpuCount,
		GPUType:    extractAccelerator(u, gpuTypeLabel),
		CPURequest: cpuReq,
		MemRequest: memReq,
		Status:     status,
	}
}

// extractAccelerator reads the accelerator value from the notebook's nodeAffinity.
func extractAccelerator(u *unstructured.Unstructured, gpuTypeLabel string) string {
	terms, _, _ := unstructured.NestedSlice(u.Object,
		"spec", "template", "spec", "affinity", "nodeAffinity",
		"requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms")
	for _, t := range terms {
		term, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		exprs, _, _ := unstructured.NestedSlice(term, "matchExpressions")
		for _, e := range exprs {
			expr, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			if key, _ := expr["key"].(string); key == gpuTypeLabel {
				if vals, _ := expr["values"].([]interface{}); len(vals) > 0 {
					if s, ok := vals[0].(string); ok {
						return s
					}
				}
			}
		}
	}
	return ""
}
