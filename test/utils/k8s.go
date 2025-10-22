package utils

import (
	"context"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"
)

func GetKubernetesClient() (*kubernetes.Clientset, error) {
	// Get kubeconfig path from env var or use default
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}

	// Use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func GetKubeconfigFromConfig(cfg *rest.Config) ([]byte, error) {
	kubeconfig, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"envtest": {
				Server:                   cfg.Host,
				CertificateAuthorityData: cfg.CAData,
				InsecureSkipTLSVerify:    cfg.Insecure,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"default": {
				ClientCertificateData: cfg.CertData,
				ClientKeyData:         cfg.KeyData,
				Token:                 cfg.BearerToken,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"envtest": {
				Cluster:  "envtest",
				AuthInfo: "default",
			},
		},
		CurrentContext: "envtest",
	})
	if err != nil {
		return nil, err
	}

	return kubeconfig, nil
}

func GetKubeConfig() (*rest.Config, error) {
	// Try to use in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fallback to kubeconfig file if not running in cluster
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// ApplyFromYAML applies a Kubernetes resource from a YAML file
func ApplyFromYAML(config *rest.Config, filePath string, namespace string, gvr schema.GroupVersionResource) error {
	// Read YAML file
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Convert YAML to unstructured
	obj := &unstructured.Unstructured{}
	jsonData, err := yaml.YAMLToJSON(yamlFile)
	if err != nil {
		return err
	}
	if err := obj.UnmarshalJSON(jsonData); err != nil {
		return err
	}

	// Get dynamic client using provided config
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}

	// Create the resource
	_, err = dynamicClient.Resource(gvr).Namespace(namespace).Create(
		context.Background(),
		obj,
		metav1.CreateOptions{},
	)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// DeleteFromYAML deletes a Kubernetes resource specified in a YAML file
func DeleteFromYAML(config *rest.Config, filePath string, namespace string, gvr schema.GroupVersionResource) error {
	// Read YAML file
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Convert YAML to unstructured
	obj := &unstructured.Unstructured{}
	jsonData, err := yaml.YAMLToJSON(yamlFile)
	if err != nil {
		return err
	}
	if err := obj.UnmarshalJSON(jsonData); err != nil {
		return err
	}

	// Get dynamic client using provided config
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}

	// Delete the resource
	err = dynamicClient.Resource(gvr).Namespace(namespace).Delete(
		context.Background(),
		obj.GetName(),
		metav1.DeleteOptions{},
	)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}
