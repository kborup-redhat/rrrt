package cli

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func requireOpenShift(ctx context.Context, clientset *kubernetes.Clientset) error {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, "openshift-monitoring", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("this tool requires an OpenShift cluster (openshift-monitoring namespace not found): %w", err)
	}
	return nil
}
