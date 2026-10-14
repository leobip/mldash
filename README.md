# mldash

**Read-only platform visibility server for ML workloads on Kubernetes.**

Answers the question teams actually ask:

> _Who has GPUs right now, which ones are free, and is anything misconfigured?_

---

## What it is

mldash is an observability server — a dashboard informer. It:

1. **Watches the cluster passively** — reads only, never modifies anything
2. **Maintains an in-memory cache** updated in real time via:
   - Kubeflow Notebook CRD informer (push events)
   - Periodic List of nodes and pods
3. **Exposes a REST API** with processed data
4. **Serves a web dashboard** with that information

The dashboard also has a **Nodes** section: per node, GPU type and total / taken / free GPU, CPU and memory. "Taken" is the sum of container requests of all scheduled pods on the node (the scheduler's view, system pods included, not actual usage), so "free" reflects what a new notebook can really be scheduled on. The Notebooks and Workloads tables show the node each pod is on, and the footer shows the running version.

Everything on screen is **reserved capacity, not measured usage** — resource columns are labelled `GPU (req)` / `CPU (req)` / `Mem (req)` accordingly. Pod and image cells drop infrastructure noise (the ReplicaSet hash and the internal registry host) and keep the full string in the row's `title` attribute so it can still be copied into `kubectl`. The namespace section only renders Total / Available columns when at least one namespace actually has a `ResourceQuota`; otherwise it becomes **Namespace Usage** with the CPU/memory its notebooks request, instead of a table of `NA` cells.

The closest pattern is `kube-state-metrics` — a server that reads cluster state and exposes it. The difference is that mldash is focused on MLOps/GPU context: who has what GPU, what notebooks are running, what capacity is free.

### Core idea

An operational portal for AI/ML teams that answers day-to-day questions like:

- Which GPU is free right now?
- Who has an H100 and what notebook is using it?
- How much real capacity is left for a new training job?

---

## How it gets the data from the cluster

1. **Connection:** `clientcmd` loads the kubeconfig/context (same mechanism `kubectl` uses) and builds two clients from the resulting `rest.Config`:
   - `kubernetes.Clientset` — typed client, for core resources (nodes, pods, resourcequotas)
   - `dynamic.Interface` — dynamic client, for CRDs (Kubeflow Notebooks)

2. **Mechanism per resource:**

   | Resource                   | Mechanism                                              | File / function                                                                                           |
   | -------------------------- | ------------------------------------------------------ | --------------------------------------------------------------------------------------------------------- |
   | Nodes                      | Periodic `List` (30s) — no `watch` permission on nodes | `watchers/nodes.go` → `client.CoreV1().Nodes().List()`                                                    |
   | Notebooks (CRD)            | Real-time informer/watch                               | `watchers/notebooks.go` → `dynamicinformer.DynamicSharedInformerFactory` over `kubeflow.org/v1/notebooks` |
   | Pod → Node mapping         | Periodic `List` in Kubeflow namespaces                 | `watchers/podnodes.go`                                                                                    |
   | Non-notebook GPU workloads | Periodic `List`, cluster-wide (excludes Kubeflow-prefixed namespaces, already covered by the Notebook informer) | `watchers/gpuworkloads.go`                                                                                |
   | ResourceQuotas             | Periodic `List`, namespace-scoped                      | `watchers/resourcequotas.go`                                                                              |

3. **Parsing unstructured CRDs:** Notebooks come back as raw JSON (`unstructured.Unstructured`), read field-by-field with `unstructured.NestedString` / `NestedInt64` / `NestedSlice`:
   - `spec.template.spec.containers[0].resources.limits["nvidia.com/gpu"]` → GPU count
   - `spec.template.spec.containers[0].resources.requests.cpu` / `.memory` → CPU/mem request
   - `status.readyReplicas` / `status.conditions` → Running / Pending (blocked, e.g. `Unschedulable`) / Stopped
   - `metadata.annotations["notebooks.kubeflow.org/creator"]` → user
   - `spec.template.spec.affinity.nodeAffinity...matchExpressions` → GPU type (accelerator label)

4. **Multi-vendor GPU support:** the extended-resource key(s) used for GPU capacity/usage and the label used to resolve the GPU model/type are configurable via `GPU_RESOURCE_KEYS` (default `nvidia.com/gpu`) and `GPU_TYPE_LABEL` (default `accelerator`) — see `.env.example`. Defaults match single-vendor NVIDIA clusters exactly (zero behavior change); add more comma-separated keys (e.g. `nvidia.com/gpu,amd.com/gpu`) for multi-vendor clusters.

5. **Table filters:** the Notebooks table supports filtering by User, Status, and GPU Type; the Workloads table by Status (default `Running`, hides stale `Failed`/`Succeeded` pods without discarding them — the JSON APIs always return unfiltered data). Plain GET query params, no JavaScript.

### Example: tracing `GPUCount` end-to-end

Step-by-step, file and line, for how a notebook's GPU count reaches the dashboard:

1. **Connect + start watch** — `cmd/server/main.go`
   - L27-41: load kubeconfig/context → `restCfg`
   - L49: `dynClient, err := dynamic.NewForConfig(restCfg)`
   - L65: `dynFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynClient, 30*time.Second)`
   - L66: `watchers.StartNotebookWatcher(dynFactory, store)`
   - L67: `dynFactory.Start(ctx.Done())` — opens the real watch connection to the API server

2. **Resource definition (GVR)** — `internal/watchers/notebooks.go`
   - L16-20: `notebookGVR = schema.GroupVersionResource{Group: "kubeflow.org", Version: "v1", Resource: "notebooks"}`

3. **Informer setup** — `internal/watchers/notebooks.go`
   - L23: `func StartNotebookWatcher(...)`
   - L24: `informer := factory.ForResource(notebookGVR).Informer()`

4. **Extract the value from raw JSON** — `internal/watchers/notebooks.go`
   - L52: `func buildNotebookInfo(u *unstructured.Unstructured) models.NotebookInfo`
   - L67: `gpuStr, _, _ := unstructured.NestedString(c, "resources", "limits", "nvidia.com/gpu")`
   - L69: `gpuCount, _ = strconv.ParseInt(gpuStr, 10, 64)` ← `GPUCount` is born here

5. **Store in the in-memory cache** — `internal/cache/store.go`
   - L56-59: `func (s *Store) SetNotebook(nb models.NotebookInfo)` → `s.notebooks[nb.Namespace+"/"+nb.Name] = nb`

6. **Read on HTTP request** — `internal/api/handlers.go`
   - L42: `func (h *Handler) Index(...)`
   - L43: `notebooks := h.store.GetNotebooks()`
   - L62-68: `data := PageData{ ..., Notebooks: notebooks, ... }`

7. **Render in the template** — `web/templates/index.html`
   - L259: `<td>{{.GPUCount}}</td>`

---

## What it is NOT

| Concept           | Difference                                          |
| ----------------- | --------------------------------------------------- |
| Operator          | Manages CRDs, reconciles state, acts on the cluster |
| Controller        | Makes decisions and modifies resources              |
| Proxy             | Does not intermediate traffic                       |
| Admission webhook | Does not validate or mutate resources               |

---

## The gap

Operational GPU data is often split across Kubernetes, Kubeflow and workload-specific tools:

| Tool               | Observability | GPUs                   | Kubeflow           | Global view | MLOps   |
| ------------------ | ------------- | ---------------------- | ------------------ | ----------- | ------- |
| Prometheus/Grafana | ✅            | partial (VRAM %, temp) | ❌                 | ❌          | ❌      |
| kube-state-metrics | ✅            | ❌                     | ❌                 | ❌          | ❌      |
| Kubeflow dashboard | partial       | ❌                     | own notebooks only | ❌          | ❌      |
| ClearML            | partial       | own agents only        | ❌                 | ❌          | partial |
| kubectl            | manual        | manual                 | manual             | ❌          | ❌      |
| **mldash**         | ✅            | ✅                     | ✅                 | ✅          | ✅      |

**Prometheus/Grafana** — performance metrics (VRAM usage, temperature, utilization %) but does not know which user has which GPU, what notebook is running, or how many GPUs are free for a new job.

**ClearML** — tracks experiments visible to its own agents, but does not provide a cluster-wide view of Kubeflow Notebooks and arbitrary Kubernetes workloads.

**Kubeflow dashboard** — shows a user's own notebooks, not a global resource view across teams.

mldash currently correlates Kubernetes nodes and pods with Kubeflow Notebooks. Direct integrations with additional MLOps platforms are future work.

---

## API

```bash
# GPU inventory — total / used / available by type
curl http://localhost:8080/api/gpus

# All notebooks — user, GPU, CPU, memory, status
curl http://localhost:8080/api/notebooks

# Non-notebook GPU workloads (model serving, inference, etc.)
curl http://localhost:8080/api/workloads

# CPU and memory quota status by namespace
curl http://localhost:8080/api/quotas

# Nodes — allocatable vs requested GPU / CPU (millicores) / memory (bytes)
curl http://localhost:8080/api/nodes

# Health check
curl http://localhost:8080/health
```

With `jq`:

```bash
# GPU summary — one line per type
curl -s http://localhost:8080/api/gpus \
  | jq '.[] | "\(.gpu_type): \(.used)/\(.total) used, \(.available) free"'

# Running notebooks with GPU
curl -s http://localhost:8080/api/notebooks \
  | jq '[.[] | select(.status=="Running" and .gpu_count>0)] | .[] | "\(.user) → \(.name) → \(.gpu_count)x \(.gpu_type)"'

# Workload status
curl -s http://localhost:8080/api/workloads \
  | jq '.[] | "\(.status) \(.name) → \(.gpu_count)x \(.gpu_type)"'
```

---

## Stack

- **Backend:** Go + client-go + dynamic client + chi router
- **Frontend:** Go templates + inline CSS custom properties, 30s auto-refresh, no JS, no npm, no build step. Design tokens and interface conventions are recorded in [`.interface-design/system.md`](.interface-design/system.md) — text hierarchy never relies on grays below AA contrast, numbers are tabular, and red means *depleted* (`available == 0`), never *in use*.
- **Deploy:** single binary, `go:embed` assets, container image and generic read-only RBAC example
- **Data sources:** Kubeflow CRD informer, periodic node/pod List

---

## Quick start

Configuration is loaded from a `.env` file in the project root (see `.env.example`).
No CLI flags — all config is via environment variables.

```bash
# Copy the example and adjust values
cp .env.example .env

# Run locally (reads .env automatically)
go run ./cmd/server

# Open dashboard
open http://localhost:8080
```

> A multi-stage Dockerfile and a generic read-only RBAC example are included in the repository. Environment-specific deployment configuration belongs in the target platform's deployment repository.

---

## Roadmap

- HTMX partial updates (replace full-page meta refresh)
- Explicit empty states for sections that currently hide themselves when they have no rows
- Generic Deployment and Service examples
- Versioned container releases pinned by digest

---

## License

No license has been selected yet. This repository is currently shared for learning, portfolio, and reference purposes.
