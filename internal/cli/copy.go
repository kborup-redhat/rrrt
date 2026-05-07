package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func copyFromPod(ctx context.Context, config *rest.Config, clientset *kubernetes.Clientset, namespace, podName, srcPath, destPath string) error {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: []string{"cat", srcPath},
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating executor: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer func() { _ = out.Close() }()

	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: out,
		Stderr: io.Discard,
	}); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("streaming file: %w", err)
	}

	return nil
}
