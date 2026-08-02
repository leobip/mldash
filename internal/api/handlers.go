package api

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"time"

	mlcache "github.com/leobip/mldash/internal/cache"
	"github.com/leobip/mldash/internal/models"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Handler holds the dependencies for the http handlers, including the cache store.
type Handler struct {
	store *mlcache.Store
	tmpl  *template.Template
}

// NewHandler creates a new Handler instance with the provided cache store.
func NewHandler(store *mlcache.Store, tmpl *template.Template) *Handler {
	return &Handler{store: store, tmpl: tmpl}
}

// PageData holds all data for the dashboard page, including GPUs, nodes, notebooks, and workloads.
type PageData struct {
	GPUs      []models.GPUInfo
	Notebooks []models.NotebookInfo
	Workloads []models.GPUWorkload
	Quotas    []models.NamespaceQuota
	UpdatedAt string
}

// HealthCheck responds with 200
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Index renders the dashboard HTML page.
func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	notebooks := h.store.GetNotebooks()

	// resolve GPU type for notebooks without nodeAffinity
	for i := range notebooks {
		if notebooks[i].GPUType == "" && notebooks[i].GPUCount > 0 {
			notebooks[i].GPUType = h.store.ResolveGPUType(notebooks[i].Namespace, notebooks[i].Name)
		}
	}

	// Sort notebooks by Status and name for consistent display
	sort.Slice(notebooks, func(i, j int) bool {
		if notebooks[i].Status != notebooks[j].Status {
			return notebooks[i].Status < notebooks[j].Status
		}
		return notebooks[i].GPUCount < notebooks[j].GPUCount
	})

	quotas := h.computeNamespaceQuotas()

	data := PageData{
		GPUs:      h.computeGPUs(),
		Notebooks: notebooks,
		Workloads: h.store.GetWorkloads(),
		Quotas:    quotas,
		UpdatedAt: time.Now().Format(time.RFC1123),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.Execute(w, data); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

// GetGPUs returns GPU inventory as JSON.
func (h *Handler) GetGPUs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.computeGPUs())
}

// GetNotebooks returns all notebooks as JSON.
func (h *Handler) GetNotebooks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetNotebooks())
}

// GetNodes returns all nodes as JSON.
func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetNodes())
}

// GetGPUWorkloads returns non-notebook GPU workloads as JSON.
func (h *Handler) GetGPUWorkloads(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetWorkloads())
}

// GetNamespaceQuotas returns ResourceQuota status per namespace as JSON.
func (h *Handler) GetNamespaceQuotas(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.computeNamespaceQuotas())
}

// computeGPUs builds the GPU summary from nodes + notebooks + workloads.
func (h *Handler) computeGPUs() []models.GPUInfo {
	nodes := h.store.GetNodes()
	notebooks := h.store.GetNotebooks()
	workloads := h.store.GetWorkloads()

	nodeAccelerator := map[string]string{}
	totalByType := map[string]int64{}
	for _, n := range nodes {
		if n.Accelerator != "" {
			nodeAccelerator[n.Name] = n.Accelerator
			totalByType[n.Accelerator] += n.GPUTotal
		}
	}

	usedByType := map[string]int64{}
	for _, nb := range notebooks {
		if nb.Status != "Running" || nb.GPUCount == 0 {
			continue
		}
		gpuType := nb.GPUType
		if gpuType == "" {
			gpuType = h.store.ResolveGPUType(nb.Namespace, nb.Name)
		}
		if gpuType != "" {
			usedByType[gpuType] += nb.GPUCount
		}
	}
	for _, w := range workloads {
		if w.Status == "Running" && w.GPUCount > 0 && w.GPUType != "" {
			usedByType[w.GPUType] += w.GPUCount
		}
	}

	gpus := make([]models.GPUInfo, 0, len(totalByType))
	for gpuType, total := range totalByType {
		used := usedByType[gpuType]
		gpus = append(gpus, models.GPUInfo{
			GPUType:   gpuType,
			Total:     total,
			Used:      used,
			Available: total - used,
		})
	}
	sort.Slice(gpus, func(i, j int) bool { return gpus[i].GPUType < gpus[j].GPUType })
	return gpus
}

// computeNamespaceQuotas combines the raw ResourceQuota hard limits (from the
// store, watcher-populated) with live notebook usage (Running only) to build
// the final per-namespace CPU/Mem view. When no ResourceQuota is configured,
// Total and Available are reported as "NA".
func (h *Handler) computeNamespaceQuotas() []models.NamespaceQuota {
	rawByNS := map[string]models.NamespaceQuota{}
	for _, q := range h.store.GetQuotas() {
		rawByNS[q.Namespace] = q
	}

	type usage struct{ cpu, mem resource.Quantity }
	usedByNS := map[string]*usage{}
	for _, nb := range h.store.GetNotebooks() {
		if nb.Status != "Running" {
			continue
		}
		u, ok := usedByNS[nb.Namespace]
		if !ok {
			u = &usage{}
			usedByNS[nb.Namespace] = u
		}
		if nb.CPURequest != "" {
			if q, err := resource.ParseQuantity(nb.CPURequest); err == nil {
				u.cpu.Add(q)
			}
		}
		if nb.MemRequest != "" {
			if q, err := resource.ParseQuantity(nb.MemRequest); err == nil {
				u.mem.Add(q)
			}
		}
	}

	seen := map[string]bool{}
	namespaces := make([]string, 0)
	for ns := range rawByNS {
		seen[ns] = true
		namespaces = append(namespaces, ns)
	}
	for ns := range usedByNS {
		if !seen[ns] {
			seen[ns] = true
			namespaces = append(namespaces, ns)
		}
	}

	result := make([]models.NamespaceQuota, 0, len(namespaces))
	for _, ns := range namespaces {
		u := usedByNS[ns]
		if u == nil {
			u = &usage{}
		}

		raw := rawByNS[ns]
		q := models.NamespaceQuota{
			Namespace: ns,
			CPUUsed:   u.cpu.String(),
			MemUsed:   u.mem.String(),
		}

		if raw.Configured {
			totalCPU, _ := resource.ParseQuantity(raw.CPUTotal)
			totalMem, _ := resource.ParseQuantity(raw.MemTotal)
			availCPU := totalCPU.DeepCopy()
			availCPU.Sub(u.cpu)
			availMem := totalMem.DeepCopy()
			availMem.Sub(u.mem)

			q.Configured = true
			q.CPUTotal = totalCPU.String()
			q.MemTotal = totalMem.String()
			q.CPUAvailable = availCPU.String()
			q.MemAvailable = availMem.String()
		} else {
			q.Configured = false
			q.Error = raw.Error
			q.CPUTotal = "NA"
			q.MemTotal = "NA"
			q.CPUAvailable = "NA"
			q.MemAvailable = "NA"
		}

		result = append(result, q)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Namespace < result[j].Namespace })
	return result
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}
