package api

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	mlcache "github.com/leobip/mldash/internal/cache"
	"github.com/leobip/mldash/internal/models"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Handler holds the dependencies for the http handlers, including the cache store.
type Handler struct {
	store            *mlcache.Store
	tmpl             *template.Template
	kubeflowNSPrefix string
}

// NewHandler creates a new Handler instance with the provided cache store.
// kubeflowNSPrefix is used to identify Kubeflow user namespaces when
// aggregating the Namespace Quotas total (GPU workload namespaces excluded).
func NewHandler(store *mlcache.Store, tmpl *template.Template, kubeflowNSPrefix string) *Handler {
	return &Handler{store: store, tmpl: tmpl, kubeflowNSPrefix: kubeflowNSPrefix}
}

// PageData holds all data for the dashboard page, including GPUs, nodes, notebooks, and workloads.
type PageData struct {
	GPUs             []models.GPUInfo
	Notebooks        []models.NotebookInfo
	NotebookUsers    []string
	NotebookGPUTypes []string
	NotebookStatus   string
	NotebookUser     string
	NotebookGPUType  string
	Workloads        []models.GPUWorkload
	HasWorkloads     bool
	WorkloadStatus   string
	Quotas           []models.NamespaceQuota
	QuotasTotal      models.NamespaceQuota
	UpdatedAt        string
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

	// filter option lists come from the full (unfiltered) notebook set
	userSet := map[string]bool{}
	gpuTypeSet := map[string]bool{}
	for _, nb := range notebooks {
		if nb.User != "" {
			userSet[nb.User] = true
		}
		if nb.GPUType != "" {
			gpuTypeSet[nb.GPUType] = true
		}
	}

	nbStatus := r.URL.Query().Get("nb_status")
	if nbStatus == "" {
		nbStatus = "Running"
	}
	nbUser := r.URL.Query().Get("nb_user")
	if nbUser == "" {
		nbUser = "All"
	}
	nbGPUType := r.URL.Query().Get("nb_gputype")
	if nbGPUType == "" {
		nbGPUType = "All"
	}

	filteredNotebooks := make([]models.NotebookInfo, 0, len(notebooks))
	for _, nb := range notebooks {
		if nbStatus != "All" && nb.Status != nbStatus {
			continue
		}
		if nbUser != "All" && nb.User != nbUser {
			continue
		}
		if nbGPUType != "All" && nb.GPUType != nbGPUType {
			continue
		}
		filteredNotebooks = append(filteredNotebooks, nb)
	}

	// Sort notebooks by Status and name for consistent display
	sort.Slice(filteredNotebooks, func(i, j int) bool {
		if filteredNotebooks[i].Status != filteredNotebooks[j].Status {
			return filteredNotebooks[i].Status < filteredNotebooks[j].Status
		}
		return filteredNotebooks[i].GPUCount < filteredNotebooks[j].GPUCount
	})

	quotas, quotasTotal := h.computeNamespaceQuotas()

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "Running"
	}
	allWorkloads := h.store.GetWorkloads()
	workloads := allWorkloads
	if status != "All" {
		workloads = make([]models.GPUWorkload, 0, len(allWorkloads))
		for _, w := range allWorkloads {
			if w.Status == status {
				workloads = append(workloads, w)
			}
		}
	}

	data := PageData{
		GPUs:             h.computeGPUs(),
		Notebooks:        filteredNotebooks,
		NotebookUsers:    sortedKeys(userSet),
		NotebookGPUTypes: sortedKeys(gpuTypeSet),
		NotebookStatus:   nbStatus,
		NotebookUser:     nbUser,
		NotebookGPUType:  nbGPUType,
		Workloads:        workloads,
		HasWorkloads:     len(allWorkloads) > 0,
		WorkloadStatus:   status,
		Quotas:           quotas,
		QuotasTotal:      quotasTotal,
		UpdatedAt:        time.Now().Format(time.RFC1123),
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
	quotas, _ := h.computeNamespaceQuotas()
	writeJSON(w, quotas)
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

	usedByNotebooksType := map[string]int64{}
	for _, nb := range notebooks {
		if nb.Status != "Running" || nb.GPUCount == 0 {
			continue
		}
		gpuType := nb.GPUType
		if gpuType == "" {
			gpuType = h.store.ResolveGPUType(nb.Namespace, nb.Name)
		}
		if gpuType != "" {
			usedByNotebooksType[gpuType] += nb.GPUCount
		}
	}
	usedByWorkloadsType := map[string]int64{}
	for _, w := range workloads {
		if w.Status == "Running" && w.GPUCount > 0 && w.GPUType != "" {
			usedByWorkloadsType[w.GPUType] += w.GPUCount
		}
	}

	gpus := make([]models.GPUInfo, 0, len(totalByType))
	for gpuType, total := range totalByType {
		usedNB := usedByNotebooksType[gpuType]
		usedWL := usedByWorkloadsType[gpuType]
		used := usedNB + usedWL
		gpus = append(gpus, models.GPUInfo{
			GPUType:         gpuType,
			Total:           total,
			Used:            used,
			UsedByNotebooks: usedNB,
			UsedByWorkloads: usedWL,
			Available:       total - used,
		})
	}
	sort.Slice(gpus, func(i, j int) bool { return gpus[i].GPUType < gpus[j].GPUType })
	return gpus
}

// computeNamespaceQuotas combines the raw ResourceQuota hard limits (from the
// store, watcher-populated) with live notebook usage (Running only) to build
// the final per-namespace CPU/Mem view. When no ResourceQuota is configured,
// Total and Available are reported as "NA". It also returns an aggregate
// TOTAL row across Kubeflow namespaces only (matching kubeflowNSPrefix),
// excluding GPU workload namespaces (e.g. rme-llm) that are not for notebooks.
func (h *Handler) computeNamespaceQuotas() ([]models.NamespaceQuota, models.NamespaceQuota) {
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
	var totalCPUUsed, totalMemUsed, totalCPUHard, totalMemHard resource.Quantity
	totalAny, totalConfigured := false, true

	for _, ns := range namespaces {
		u := usedByNS[ns]
		if u == nil {
			u = &usage{}
		}

		raw := rawByNS[ns]
		q := models.NamespaceQuota{
			Namespace: ns,
			CPUUsed:   formatCPUCores(u.cpu),
			MemUsed:   u.mem.String(),
		}

		isKubeflowNS := strings.HasPrefix(ns, h.kubeflowNSPrefix)

		if raw.Configured {
			totalCPU, _ := resource.ParseQuantity(raw.CPUTotal)
			totalMem, _ := resource.ParseQuantity(raw.MemTotal)
			availCPU := totalCPU.DeepCopy()
			availCPU.Sub(u.cpu)
			availMem := totalMem.DeepCopy()
			availMem.Sub(u.mem)

			q.Configured = true
			q.CPUTotal = formatCPUCores(totalCPU)
			q.MemTotal = totalMem.String()
			q.CPUAvailable = formatCPUCores(availCPU)
			q.MemAvailable = availMem.String()

			if isKubeflowNS {
				totalCPUHard.Add(totalCPU)
				totalMemHard.Add(totalMem)
			}
		} else {
			q.Configured = false
			q.Error = raw.Error
			q.CPUTotal = "NA"
			q.MemTotal = "NA"
			q.CPUAvailable = "NA"
			q.MemAvailable = "NA"

			if isKubeflowNS {
				totalConfigured = false
			}
		}

		if isKubeflowNS {
			totalAny = true
			totalCPUUsed.Add(u.cpu)
			totalMemUsed.Add(u.mem)
		}

		result = append(result, q)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Namespace < result[j].Namespace })

	total := models.NamespaceQuota{
		Namespace: "TOTAL (Kubeflow)",
		CPUUsed:   formatCPUCores(totalCPUUsed),
		MemUsed:   totalMemUsed.String(),
	}
	if totalAny && totalConfigured {
		availCPU := totalCPUHard.DeepCopy()
		availCPU.Sub(totalCPUUsed)
		availMem := totalMemHard.DeepCopy()
		availMem.Sub(totalMemUsed)

		total.Configured = true
		total.CPUTotal = formatCPUCores(totalCPUHard)
		total.MemTotal = totalMemHard.String()
		total.CPUAvailable = formatCPUCores(availCPU)
		total.MemAvailable = availMem.String()
	} else {
		total.Configured = false
		total.CPUTotal = "NA"
		total.MemTotal = "NA"
		total.CPUAvailable = "NA"
		total.MemAvailable = "NA"
	}

	return result, total
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

// sortedKeys returns the keys of a set map sorted alphabetically, used to
// build stable dropdown option lists.
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatCPUCores(q resource.Quantity) string {
	milli := q.MilliValue()
	if milli%1000 == 0 {
		return strconv.FormatInt(milli/1000, 10)
	}
	return strconv.FormatFloat(float64(milli)/1000.0, 'f', -1, 64)
}
