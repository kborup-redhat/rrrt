package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type RunConfig struct {
	Namespaces    []string
	Output        string
	LookbackDays  int
	Image         string
	ConsoleURL    string
	IncludeOS     bool
	KeepNamespace bool
	Timeout       time.Duration
	Version       string
}

func Run(ctx context.Context, config *rest.Config, clientset *kubernetes.Clientset, cfg RunConfig) error {
	randBytes := make([]byte, 4)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("generating random name: %w", err)
	}
	nsName := "rrrt-" + hex.EncodeToString(randBytes)

	if cfg.ConsoleURL == "" {
		url, err := detectConsoleURL(ctx, clientset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not auto-detect console URL: %v\n", err)
		} else {
			cfg.ConsoleURL = url
		}
	}

	if cfg.Image == "" {
		tag := cfg.Version
		if tag == "" || tag == "dev" {
			tag = "latest"
		}
		cfg.Image = "quay.io/kborup/rrrt:" + tag
	}

	prometheusURL := "https://thanos-querier.openshift-monitoring.svc:9091"

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	fmt.Printf("Creating namespace %s...\n", nsName)
	if err := createNamespace(ctx, clientset, nsName); err != nil {
		return fmt.Errorf("creating namespace: %w", err)
	}

	cleanup := NewCleanup(clientset, nsName, nil, cfg.KeepNamespace)
	cleanup.RegisterSignalHandler(cancel)

	if err := createServiceAccount(ctx, clientset, nsName); err != nil {
		cleanup.Run()
		return fmt.Errorf("creating service account: %w", err)
	}

	if err := ensureClusterRole(ctx, clientset); err != nil {
		cleanup.Run()
		return fmt.Errorf("ensuring cluster role: %w", err)
	}

	crbNames, err := createClusterRoleBindings(ctx, clientset, nsName)
	if err != nil {
		cleanup.Run()
		return fmt.Errorf("creating cluster role bindings: %w", err)
	}
	cleanup.crbNames = crbNames

	job := buildJob(JobConfig{
		Namespace:     nsName,
		Image:         cfg.Image,
		Namespaces:    cfg.Namespaces,
		LookbackDays:  cfg.LookbackDays,
		ConsoleURL:    cfg.ConsoleURL,
		IncludeOS:     cfg.IncludeOS,
		PrometheusURL: prometheusURL,
		Timeout:       cfg.Timeout,
	})

	fmt.Println("Creating analyzer Job...")
	_, err = clientset.BatchV1().Jobs(nsName).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		cleanup.Run()
		return fmt.Errorf("creating job: %w", err)
	}

	podName, err := waitForPod(ctx, clientset, nsName, cfg.Timeout)
	if err != nil {
		cleanup.Run()
		return fmt.Errorf("waiting for pod: %w", err)
	}

	if err := streamLogs(ctx, clientset, nsName, podName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: log streaming interrupted: %v\n", err)
	}

	if err := waitForJobCompletion(ctx, clientset, nsName, cfg.Timeout); err != nil {
		cleanup.Run()
		return fmt.Errorf("job failed: %w", err)
	}

	outputPath := cfg.Output
	if outputPath == "" {
		outputPath = fmt.Sprintf("rrrt-report-%s.pdf", time.Now().Format("2006-01-02T1504"))
	}

	fmt.Println("Downloading report...")
	if err := copyFromPod(ctx, config, clientset, nsName, podName, "/output/report.pdf", outputPath); err != nil {
		cleanup.Run()
		return fmt.Errorf("copying report: %w", err)
	}

	cleanup.Run()

	fmt.Printf("Report saved to %s\n", outputPath)
	return nil
}

func waitForPod(ctx context.Context, clientset *kubernetes.Clientset, namespace string, timeout time.Duration) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		pods, err := clientset.CoreV1().Pods(namespace).List(timeoutCtx, metav1.ListOptions{
			LabelSelector: "job-name=rrrt-analyzer",
		})
		if err != nil {
			return "", err
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning ||
				pod.Status.Phase == corev1.PodSucceeded ||
				pod.Status.Phase == corev1.PodFailed {
				return pod.Name, nil
			}
		}

		time.Sleep(2 * time.Second)

		select {
		case <-timeoutCtx.Done():
			return "", fmt.Errorf("timed out waiting for pod")
		default:
		}
	}
}

func streamLogs(ctx context.Context, clientset *kubernetes.Clientset, namespace, podName string) error {
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Follow: true,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		line := scanner.Text()

		var progress struct {
			Phase     string `json:"phase"`
			Namespace string `json:"namespace"`
			Resource  string `json:"resource"`
			Index     int    `json:"index"`
			Total     int    `json:"total"`
			Status    string `json:"status"`
		}

		if json.Unmarshal([]byte(line), &progress) == nil && progress.Phase != "" {
			if progress.Resource != "" {
				fmt.Printf("[%d/%d] Analyzing %s %s/%s...\n", progress.Index, progress.Total, progress.Phase, progress.Namespace, progress.Resource)
			} else {
				fmt.Printf("[%d/%d] %s %s...\n", progress.Index, progress.Total, progress.Status, progress.Namespace)
			}
		} else {
			fmt.Println(line)
		}
	}

	return scanner.Err()
}

func waitForJobCompletion(ctx context.Context, clientset *kubernetes.Clientset, namespace string, timeout time.Duration) error {
	watcher, err := clientset.BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=rrrt-analyzer",
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}
			if event.Type == watch.Modified {
				job, ok := event.Object.(*batchv1.Job)
				if !ok {
					continue
				}
				for _, cond := range job.Status.Conditions {
					if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
						return nil
					}
					if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
						return fmt.Errorf("job failed: %s", cond.Message)
					}
				}
			}
		case <-timeoutCh:
			return fmt.Errorf("timed out waiting for job completion")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
