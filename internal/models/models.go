package models

// GPUInfo represents GPU inventory summary per accelerator type.
type GPUInfo struct {
	GPUType   string `json:"gpu_type"`
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
	Available int64  `json:"available"`
}

// NodeInfo represents a node with its GPU capacity.
type NodeInfo struct {
	Name        string `json:"name"`
	Accelerator string `json:"accelerator"` // e.g. nvidia-h100, nvidia-l4
	GPUTotal    int64  `json:"gpu_total"`
	Ready       bool   `json:"ready"`
}

// NotebookInfo represents a Kubeflow Notebook and its resource requests.
type NotebookInfo struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	User       string `json:"user"`
	GPUCount   int64  `json:"gpu_count"`
	GPUType    string `json:"gpu_type"`
	CPURequest string `json:"cpu_request"`
	MemRequest string `json:"mem_request"`
	Status     string `json:"status"` // Running / Stopped
}

// NamespaceQuota represents CPU/memory ResourceQuota status for a namespace.
// Configured is false when the namespace has no ResourceQuota object defined.
// Error is set to "forbidden" when we lack RBAC to read resourcequotas in the namespace.
type NamespaceQuota struct {
	Namespace    string `json:"namespace"`
	Configured   bool   `json:"configured"`
	Error        string `json:"error,omitempty"`
	CPUTotal     string `json:"cpu_total,omitempty"`
	CPUUsed      string `json:"cpu_used,omitempty"`
	CPUAvailable string `json:"cpu_available,omitempty"`
	MemTotal     string `json:"mem_total,omitempty"`
	MemUsed      string `json:"mem_used,omitempty"`
	MemAvailable string `json:"mem_available,omitempty"`
}

// GPUWorkload represents a non-notebook pod using GPUs.
type GPUWorkload struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Image      string `json:"image"`
	GPUCount   int64  `json:"gpu_count"`
	GPUType    string `json:"gpu_type"`
	CPURequest string `json:"cpu_request"`
	MemRequest string `json:"mem_request"`
	Status     string `json:"status"`
	NodeName   string `json:"node_name"`
}
