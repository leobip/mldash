package watchers

import corev1 "k8s.io/api/core/v1"

// SumGPU adds up the qty of GPU resources in the given ResourceList, using the provided resource keys (e.g. "nvidia.com/gpu").
func SumGPU(rl corev1.ResourceList, gpuResourceKeys []string) int64 {
	var total int64
	for _, key := range gpuResourceKeys {
		if qty, ok := rl[corev1.ResourceName(key)]; ok {
			n, _ := qty.AsInt64()
			total += n
		}
	}
	return total
}
