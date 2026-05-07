package cli

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"gopkg.in/yaml.v3"
)

func detectConsoleURL(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
	cm, err := clientset.CoreV1().ConfigMaps("openshift-console").Get(ctx, "console-config", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("reading console-config: %w", err)
	}

	consoleConfig, ok := cm.Data["console-config.yaml"]
	if !ok {
		return "", fmt.Errorf("console-config.yaml not found in ConfigMap")
	}

	var config struct {
		ClusterInfo struct {
			ConsoleBaseAddress string `yaml:"consoleBaseAddress"`
		} `yaml:"clusterInfo"`
	}

	if err := yaml.Unmarshal([]byte(consoleConfig), &config); err != nil {
		return "", fmt.Errorf("parsing console config: %w", err)
	}

	if config.ClusterInfo.ConsoleBaseAddress == "" {
		return "", fmt.Errorf("consoleBaseAddress not found")
	}

	return config.ClusterInfo.ConsoleBaseAddress, nil
}
