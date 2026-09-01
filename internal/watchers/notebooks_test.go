package watchers

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildNotebookInfo_BlockedNotebookIsPending(t *testing.T) {
	// Mirrors the real tesy-thom case seen on 2026-08-25: readyReplicas == 0
	// but status.conditions has a PodScheduled=False entry (Unschedulable).
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":        "tesy-thom",
			"namespace":   "rme-kubeflow-leoncio-lopez-pelayo",
			"annotations": map[string]interface{}{},
		},
		"status": map[string]interface{}{
			"readyReplicas": int64(0),
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "PodScheduled",
					"status":  "False",
					"reason":  "Unschedulable",
					"message": "0/10 nodes are available: 2 Insufficient nvidia.com/gpu.",
				},
			},
		},
	}}

	info := buildNotebookInfo(u, []string{"nvidia.com/gpu"}, "accelerator")

	if info.Status != "Pending" {
		t.Fatalf("expected status Pending, got %q", info.Status)
	}
	if info.StatusReason != "0/10 nodes are available: 2 Insufficient nvidia.com/gpu." {
		t.Fatalf("unexpected status reason: %q", info.StatusReason)
	}
}

func TestBuildNotebookInfo_StoppedNotebookHasNoConditions(t *testing.T) {
	// Mirrors the real orec case: user-stopped notebook, empty conditions.
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":        "orec",
			"namespace":   "rme-kubeflow-someuser",
			"annotations": map[string]interface{}{},
		},
		"status": map[string]interface{}{
			"readyReplicas": int64(0),
			"conditions":    []interface{}{},
		},
	}}

	info := buildNotebookInfo(u, []string{"nvidia.com/gpu"}, "accelerator")

	if info.Status != "Stopped" {
		t.Fatalf("expected status Stopped, got %q", info.Status)
	}
	if info.StatusReason != "" {
		t.Fatalf("expected empty status reason, got %q", info.StatusReason)
	}
}

func TestBuildNotebookInfo_RunningNotebook(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":        "running-nb",
			"namespace":   "rme-kubeflow-someuser",
			"annotations": map[string]interface{}{},
		},
		"status": map[string]interface{}{
			"readyReplicas": int64(1),
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "PodScheduled",
					"status": "False",
					"reason": "Unschedulable",
				},
			},
		},
	}}

	info := buildNotebookInfo(u, []string{"nvidia.com/gpu"}, "accelerator")

	if info.Status != "Running" {
		t.Fatalf("expected status Running (readyReplicas takes priority), got %q", info.Status)
	}
}
