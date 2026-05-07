package cli

import (
	"archive/tar"
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
			Command: []string{"tar", "cf", "-", "-C", "/", srcPath[1:]},
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating executor: %w", err)
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdout: pw,
			Stderr: os.Stderr,
		})
		if err != nil {
			pw.CloseWithError(err)
		}
	}()

	tr := tar.NewReader(pr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		if header.Typeflag == tar.TypeReg {
			out, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("writing output file: %w", err)
			}
			out.Close()
			return nil
		}
	}

	return fmt.Errorf("file not found in tar stream: %s", srcPath)
}
