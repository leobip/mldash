---
layout: single
classes: wide
title: "MLDash: building an observability dashboard for Kubeflow notebooks"
categories:
  - golang
  - kubernetes
  - mlops
tags:
  - golang
  - kubernetes
  - kubeflow
  - mlops
  - observability
  - side-project
date: "2026-07-18 09:00:00"
header:
  teaser: /assets/images/2026-07-18-mldash-teaser.png
---

For the past months I've been working on designing and deploying a full
MLOps platform: LLM models in production (code and chat), shared GPUs,
Kubeflow, ClearML + agents, all orchestrated by ArgoCD.

That's material for many articles on its own (they'll come 👀).

But this story starts in a meeting. A question, out of nowhere:

> Is there any way to know who's using which GPU, how much CPU and memory,
> across Kubeflow notebooks?

That question stuck with me.

![mldash — The Question and the Idea](/assets/images/2026-07-18-mldash.jpg)

*Illustrative image generated with fictional users, workloads, nodes, and GPU values.*

So I started digging on my own to see if something like this already
existed. I looked at Kubeflow. I looked at Kubernetes. I looked at
Prometheus/Grafana. I found scattered pieces, but nothing that tied
together **GPU + user + notebook + resources** in one place.

If nobody else was going to solve it, and it was the perfect excuse to
write some Go over a weekend, why not?

A couple of days later, I had a POC running against a real cluster, with
real data — live GPU inventory, active notebooks, users identified.

**That's how mldash was born.**

An observability server, not an operator — it only reads cluster state and
exposes it. It touches nothing. Minimal permissions, deliberately designed
to be non-invasive in a cluster shared across multiple teams.

![mldash — how it reads the cluster](/assets/images/2026-07-18-mldash-architecture-eng.png)

*Generic architecture illustration; it does not represent a specific environment.*

## First design pass

After thinking through the problem, this is the basic architecture that
came out of it (I'm sure it'll change and evolve as it grows):

- A **watcher** concept: small pieces that keep an eye on specific cluster
  resources and cache what they find.
- Query each user's namespace — that's where Kubeflow notebooks actually
  live, one namespace per developer.
- Get the total GPU count available in the cluster.
- Cross both: what's there vs. what's being used.

Simple starting point. It's not the final design, but it's a design I
could actually build against.

To get there, I went through several variations of Go's client-go — the
same API `kubectl` itself talks to, just embedded directly in my own
binary. Once I confirmed a call worked, I'd refine it. Here's an early
example: listing nodes and reading how many GPUs the cluster has
allocatable, cluster-wide:

```go
// Connect to the Kubernetes cluster from inside the pod
config, err := rest.InClusterConfig()
if err != nil {
    log.Fatal("not running in cluster")
}
clientset, err := kubernetes.NewForConfig(config)

// List all nodes and sum up their GPU capacity
list, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
for _, node := range list.Items {
    gpuQ := node.Status.Allocatable["nvidia.com/gpu"]
    gpuCount, _ := gpuQ.AsInt64()
    // Store in cache...
}
```

Nothing fancy — just the same information `kubectl` would show you:

![kubectl get nodes with GPU](/assets/images/2026-07-18-mldash-kubectl-terminal.png)

*Mock terminal output with fictional node names and GPU values.*

But in Go, structured and cached so the dashboard can use it directly.

It's still a work in progress — an MVP, not a product. Still missing: a
Dockerfile, deployment manifests, code cleanup, and (honestly) getting past
my own "not ready to show this yet" stage before opening it up 😅.

Once it clears that bar, the repo goes public.

This is just getting started. I'll keep sharing the process — design
decisions, why Go, how it reads the cluster without ever modifying it, and
the bumps along the way.

**To be continued.**

## Continued — from POC to deployed MVP

> **Update — August 2, 2026:** the paragraph above describes the project at
> the time of the first draft. Since then, mldash has gained container images,
> deployment resources and a working deployment in a Kubernetes environment.

![mldash — from POC to deployed MVP](/assets/images/2026-08-02-mldash-poc-to-mvp.jpg)

*From POC to deployed MVP — containerized, published, and deployed through a
read-only GitOps workflow. Conceptual illustration; all interfaces, users,
workloads, nodes, and values are fictional.*

The first step after the POC was making the binary portable. I added a
multi-stage Dockerfile: Go compiles a static binary in the build stage, and the
runtime stage contains only that binary in a distroless image running as a
non-root user. A separate local troubleshooting image keeps tools needed for
kubeconfig-based testing out of the production image.

The next step was repeatable distribution. GitHub Actions now builds the image
with Buildx, publishes version and commit tags to GHCR, and targets both
`linux/amd64` and `linux/arm64`. The cluster can run the amd64 image while the
same release can be tested on an arm64 development machine.

Deployment stayed deliberately separate from application code. Environment
variables select the Kubeflow namespace prefix and any extra workload
namespaces. Deployment manifests choose mldash's own namespace and Kubernetes
supplies its identity through a ServiceAccount. The RBAC policy remains
read-only: list nodes and namespaces, watch Kubeflow
Notebooks, and list only the pod and ResourceQuota data needed by the
dashboard. Environment-specific DNS, ingress and authentication remain the
responsibility of the platform deploying it.

The deployment process also exposed several useful failure modes:

- A `ClusterRoleBinding` subject must reference the namespace where the
  ServiceAccount actually exists. Moving the workload without updating that
  subject leaves the pod running but unauthorized.
- A Kubernetes RBAC `apiGroup` contains only the group (`kubeflow.org`), not the
  API version (`kubeflow.org/v1`). A small-looking mismatch can prevent an
  informer from listing its initial objects.
- The HTTP server currently waits for the Notebook informer cache to sync. If
  RBAC prevents that initial sync, the entire dashboard remains unavailable
  instead of only losing the Notebook section. That is now a known resilience
  trade-off to revisit.

After correcting those issues, the original POC became a deployed MVP serving
live cluster state through the dashboard and REST API. The next development
step is making the public project safer and more reusable: keeping deployment
configuration private to each environment, removing environment-specific
assumptions, and generalizing GPU resource and accelerator-label discovery.
