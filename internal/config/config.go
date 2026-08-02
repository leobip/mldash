package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Addr             string   // HTTP listen address, e.g. ":8080"
	Kubeconfig       string   // path to kubeconfig file (empty = in-cluster)
	KubeContext      string   // kubeconfig context to use (empty = current context)
	KubeflowNSPrefix string   // prefix for Kubeflow user namespaces
	GPUNamespaces    []string // extra namespaces with GPU workloads (non-notebook)
}

// Load reads configuration from environment variables.
// If a .env file exists in the working directory it is loaded first.
func Load() Config {
	_ = godotenv.Load() // no-op if .env does not exist

	return Config{
		Addr:             ":" + getEnvOrDefault("PORT", "8080"),
		Kubeconfig:       os.Getenv("KUBECONFIG"),
		KubeContext:      os.Getenv("KUBE_CONTEXT"),
		KubeflowNSPrefix: getEnvOrDefault("KUBEFLOW_NS_PREFIX", "kubeflow-"),
		GPUNamespaces:    parseCSV(os.Getenv("GPU_NAMESPACES")),
	}
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
