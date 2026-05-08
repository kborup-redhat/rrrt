package collector

import (
	"context"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

type OVRODiscoveryResult struct {
	Detected bool
	Endpoint string
	Message  string
}

func DiscoverOVRO(ctx context.Context, clientset *kubernetes.Clientset, ovroNamespace, crdName, vmURL string) OVRODiscoveryResult {
	if !checkCRD(clientset.Discovery(), crdName) {
		return OVRODiscoveryResult{Message: fmt.Sprintf("CRD %s not found", crdName)}
	}

	if !checkNamespace(ctx, clientset, ovroNamespace) {
		return OVRODiscoveryResult{Message: fmt.Sprintf("namespace %s not found", ovroNamespace)}
	}

	if !checkVictoriaMetricsPod(ctx, clientset, ovroNamespace) {
		return OVRODiscoveryResult{Message: "VictoriaMetrics pod not running in " + ovroNamespace}
	}

	if !ProbeHealth(ctx, vmURL) {
		return OVRODiscoveryResult{Message: "VictoriaMetrics health probe failed at " + vmURL}
	}

	return OVRODiscoveryResult{
		Detected: true,
		Endpoint: vmURL,
		Message:  "OVRO detected: using VictoriaMetrics at " + vmURL,
	}
}

func checkCRD(disc discovery.DiscoveryInterface, crdName string) bool {
	_, resourceLists, err := disc.ServerGroupsAndResources()
	if err != nil {
		return false
	}
	parts := splitCRDName(crdName)
	if parts == nil {
		return false
	}
	for _, rl := range resourceLists {
		for _, r := range rl.APIResources {
			if r.Name == parts[0] {
				return true
			}
		}
	}
	return false
}

func splitCRDName(name string) []string {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return []string{name[:i], name[i+1:]}
		}
	}
	return nil
}

func checkNamespace(ctx context.Context, clientset *kubernetes.Clientset, name string) bool {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

func checkVictoriaMetricsPod(ctx context.Context, clientset *kubernetes.Clientset, namespace string) bool {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=victoriametrics",
	})
	if err != nil || len(pods.Items) == 0 {
		return false
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return true
		}
	}
	return false
}

func ProbeHealth(ctx context.Context, baseURL string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
