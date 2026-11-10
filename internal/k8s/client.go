package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClient creates a new Kubernetes clientset using either explicit kubeconfig,
// in-cluster configuration, or the default ~/.kube/config.
func NewClient(kubeconfigPath string) (kubernetes.Interface, error) {
	var (
		cfg *rest.Config
		err error
	)

	if kubeconfigPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("building config from kubeconfig %s: %w", kubeconfigPath, err)
		}
	} else {
		// 1. Try In-Cluster Config
		cfg, err = rest.InClusterConfig()
		if err != nil {
			// 2. Try default ~/.kube/config for local development
			homeDir, hErr := os.UserHomeDir()
			if hErr == nil {
				defaultKubeconfig := filepath.Join(homeDir, ".kube", "config")
				if _, sErr := os.Stat(defaultKubeconfig); sErr == nil {
					cfg, err = clientcmd.BuildConfigFromFlags("", defaultKubeconfig)
				}
			}
			if cfg == nil {
				return nil, fmt.Errorf("unable to load Kubernetes config (in-cluster or local ~/.kube/config): %w", err)
			}
		}
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	return clientset, nil
}
