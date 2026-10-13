package watchers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func podOn(node string, cpu, mem, gpu string) corev1.Pod {
	req := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
	if gpu != "" {
		req["nvidia.com/gpu"] = resource.MustParse(gpu)
	}
	return corev1.Pod{Spec: corev1.PodSpec{
		NodeName:   node,
		Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: req}}},
	}}
}

func TestSumPodRequests(t *testing.T) {
	pods := []corev1.Pod{
		podOn("n1", "500m", "1Gi", "1"),
		podOn("n1", "1", "2Gi", ""),
		podOn("n2", "2", "4Gi", "2"),
		podOn("", "8", "8Gi", "4"), // not scheduled: ignored
	}
	got := sumPodRequests(pods, []string{"nvidia.com/gpu"})

	n1 := got["n1"]
	if n1.gpu != 1 || n1.cpuMilli != 1500 || n1.memBytes != 3<<30 {
		t.Errorf("n1 = %+v", n1)
	}
	n2 := got["n2"]
	if n2.gpu != 2 || n2.cpuMilli != 2000 || n2.memBytes != 4<<30 {
		t.Errorf("n2 = %+v", n2)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(got))
	}
}
