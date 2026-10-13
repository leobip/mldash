package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/leobip/mldash/internal/api"
	"github.com/leobip/mldash/internal/cache"
	"github.com/leobip/mldash/internal/config"
	"github.com/leobip/mldash/internal/watchers"
	"github.com/leobip/mldash/web"
)

var version = "dev" // set at build time with -ldflags "-X main.version=1.2.3"

func main() {
	cfg := config.Load()

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if cfg.KubeContext != "" {
		overrides.CurrentContext = cfg.KubeContext
	}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, overrides,
	).ClientConfig()
	if err != nil {
		log.Fatalf("kubeconfig error: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		log.Fatalf("kubernetes client error: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		log.Fatalf("dynamic client error: %v", err)
	}

	store := cache.NewStore()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Node refresher: periodic List (no Watch needed)
	watchers.StartNodeRefresher(ctx, clientset, store, cfg.GPUResourceKeys, cfg.GPUTypeLabel)
	watchers.StartPodNodeRefresher(ctx, clientset, store, cfg.KubeflowNSPrefix)
	watchers.StartGPUWorkloadRefresher(ctx, clientset, store, cfg.KubeflowNSPrefix, cfg.GPUResourceKeys, cfg.GPUTypeLabel)

	// Notebook watcher: dynamic informer on kubeflow.org/v1/notebooks
	dynFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynClient, 30*time.Second)
	watchers.StartNotebookWatcher(dynFactory, store, cfg.GPUResourceKeys, cfg.GPUTypeLabel)
	dynFactory.Start(ctx.Done())

	synced := dynFactory.WaitForCacheSync(ctx.Done())
	for gvr, ok := range synced {
		if !ok {
			log.Fatalf("cache sync failed for %s", gvr)
		}
	}
	log.Println("notebook cache synced")

	// Resource quota refresher: depends on notebook namespaces being known
	watchers.StartResourceQuotaRefresher(ctx, clientset, store, cfg.GPUNamespaces)

	tmpl := template.Must(
		template.New("index.html").Funcs(template.FuncMap{
			"statusIcon": func(s string) string {
				switch s {
				case "Running":
					return "🟢"
				case "Stopped":
					return "🔴"
				case "Pending":
					return "🟡"
				default:
					return "⚪"
				}
			},
			"statusClass": func(s string) string {
				return strings.ToLower(s)
			},
			"cpuCores": func(milli int64) string {
				return fmt.Sprintf("%g", float64(milli)/1000)
			},
			"version": func() string {
				return version
			},
			"memGB": func(b int64) string {
				return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
			},
		}).ParseFS(web.Templates, "templates/index.html"),
	)
	handler := api.NewHandler(store, tmpl, cfg.KubeflowNSPrefix)
	router := api.NewRouter(handler)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	go func() {
		log.Printf("mldash dashboard ready: http://localhost%s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx) //nolint:errcheck
	log.Println("server stopped")
}
