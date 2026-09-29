package cache

import (
	"sync"

	"github.com/leobip/mldash/internal/models"
)

// Store holds the in-memory state of the cluster.
type Store struct {
	mu        sync.RWMutex
	nodes     map[string]models.NodeInfo
	notebooks map[string]models.NotebookInfo
	podNodes  map[string]string // maps pod namespace/name to node name
	workloads map[string]models.GPUWorkload
	quotas    map[string]models.NamespaceQuota // keyed by namespace
}

// NewStore initializes and returns a new Store instance.
func NewStore() *Store {
	return &Store{
		nodes:     make(map[string]models.NodeInfo),
		notebooks: make(map[string]models.NotebookInfo),
		podNodes:  make(map[string]string),
		workloads: make(map[string]models.GPUWorkload),
		quotas:    make(map[string]models.NamespaceQuota),
	}
}

// SetNode updates or adds a node's information in the store.
func (s *Store) SetNode(node models.NodeInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.Name] = node
}

// DeleteNode removes a node's information from the store.
func (s *Store) DeleteNode(node string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, node)
}

// GetNodes retrieves all nodes' information from the store.
func (s *Store) GetNodes() []models.NodeInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.NodeInfo, 0, len(s.nodes))
	for _, node := range s.nodes {
		result = append(result, node)
	}
	return result
}

// NodeAccelerator returns the accelerator type (GPU model label) for a node.
func (s *Store) NodeAccelerator(nodeName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if node, ok := s.nodes[nodeName]; ok {
		return node.Accelerator
	}
	return ""
}

// SetNotebook updates or adds a notebook's information in the store.
func (s *Store) SetNotebook(nb models.NotebookInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notebooks[nb.Namespace+"/"+nb.Name] = nb
}

// DeleteNotebook removes a notebook's information from the store.
func (s *Store) DeleteNotebook(namespace, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notebooks, namespace+"/"+name)
}

// GetNotebooks retrieves all notebooks' information from the store.
func (s *Store) GetNotebooks() []models.NotebookInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.NotebookInfo, 0, len(s.notebooks))
	for _, nb := range s.notebooks {
		result = append(result, nb)
	}
	return result
}

// NotebookNamespaces returns the distinct set of namespaces that currently have notebooks.
func (s *Store) NotebookNamespaces() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, nb := range s.notebooks {
		if !seen[nb.Namespace] {
			seen[nb.Namespace] = true
			result = append(result, nb.Namespace)
		}
	}
	return result
}

func (s *Store) SetPodNode(namespace, name, nodeName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.podNodes[namespace+"/"+name] = nodeName
}
func (s *Store) GetPodNode(namespace, name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.podNodes[namespace+"/"+name]
}

// ResolveGPUType returns the accelerator type for a notebook pod
// by looking up pod→nodeName→node.Accelerator.
// Tries both <name> and <name>-0 (StatefulSet pods).
func (s *Store) ResolveGPUType(namespace, name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodeName := s.podNodes[namespace+"/"+name]
	if nodeName == "" {
		nodeName = s.podNodes[namespace+"/"+name+"-0"]
	}
	if nodeName == "" {
		return ""
	}
	if node, ok := s.nodes[nodeName]; ok {
		return node.Accelerator
	}
	return ""
}

func (s *Store) SetWorkload(w models.GPUWorkload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workloads[w.Namespace+"/"+w.Name] = w
}
func (s *Store) DeleteWorkload(namespace, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workloads, namespace+"/"+name)
}
func (s *Store) GetWorkloads() []models.GPUWorkload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.GPUWorkload, 0, len(s.workloads))
	for _, w := range s.workloads {
		result = append(result, w)
	}
	return result
}

// SetQuota updates or adds a namespace's ResourceQuota status in the store.
func (s *Store) SetQuota(q models.NamespaceQuota) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotas[q.Namespace] = q
}

// GetQuotas retrieves all namespace quota statuses from the store.
func (s *Store) GetQuotas() []models.NamespaceQuota {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.NamespaceQuota, 0, len(s.quotas))
	for _, q := range s.quotas {
		result = append(result, q)
	}
	return result
}
