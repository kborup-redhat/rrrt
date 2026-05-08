package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Cleanup struct {
	clientset     *kubernetes.Clientset
	namespace     string
	crbNames      []string
	keepNamespace bool
}

func NewCleanup(clientset *kubernetes.Clientset, namespace string, crbNames []string, keepNamespace bool) *Cleanup {
	return &Cleanup{
		clientset:     clientset,
		namespace:     namespace,
		crbNames:      crbNames,
		keepNamespace: keepNamespace,
	}
}

func (c *Cleanup) RegisterSignalHandler(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nInterrupted. Cleaning up...")
		cancel()
		c.Run()
		os.Exit(1)
	}()
}

func (c *Cleanup) Run() {
	if c.keepNamespace {
		fmt.Fprintf(os.Stderr, "Keeping namespace %s (--keep-namespace set)\n", c.namespace)
		return
	}

	ctx := context.Background()

	// Best-effort cleanup of OVRO NetworkPolicy (created by analyzer if OVRO was detected)
	_ = c.clientset.NetworkingV1().NetworkPolicies("ovro-system").Delete(
		ctx, "allow-rrrt-to-victoriametrics", metav1.DeleteOptions{})

	for _, name := range c.crbNames {
		err := c.clientset.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete ClusterRoleBinding %s: %v\n", name, err)
		}
	}

	err := c.clientset.CoreV1().Namespaces().Delete(ctx, c.namespace, metav1.DeleteOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cleanup failed. Run manually:\n")
		fmt.Fprintf(os.Stderr, "  oc delete networkpolicy allow-rrrt-to-victoriametrics -n ovro-system\n")
		for _, name := range c.crbNames {
			fmt.Fprintf(os.Stderr, "  oc delete clusterrolebinding %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "  oc delete namespace %s\n", c.namespace)
	}
}
