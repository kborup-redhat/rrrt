package cli

import (
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JobConfig struct {
	Namespace     string
	Image         string
	Namespaces    []string
	LookbackDays  int
	ConsoleURL    string
	IncludeOS     bool
	PrometheusURL string
	Timeout       time.Duration
	NoOVRO        bool
	OVROEndpoint  string
}

func buildJob(cfg JobConfig) *batchv1.Job {
	deadlineSeconds := int64(cfg.Timeout.Seconds())
	backoffLimit := int32(0)

	env := []corev1.EnvVar{
		{Name: "RRRT_LOOKBACK_DAYS", Value: fmt.Sprintf("%d", cfg.LookbackDays)},
		{Name: "RRRT_CONSOLE_URL", Value: cfg.ConsoleURL},
		{Name: "RRRT_INCLUDE_OPENSHIFT", Value: fmt.Sprintf("%t", cfg.IncludeOS)},
		{Name: "RRRT_PROMETHEUS_URL", Value: cfg.PrometheusURL},
	}

	if len(cfg.Namespaces) > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "RRRT_NAMESPACES",
			Value: strings.Join(cfg.Namespaces, ","),
		})
	}

	if cfg.NoOVRO {
		env = append(env, corev1.EnvVar{Name: "RRRT_NO_OVRO", Value: "true"})
	}
	if cfg.OVROEndpoint != "" {
		env = append(env, corev1.EnvVar{Name: "RRRT_OVRO_ENDPOINT", Value: cfg.OVROEndpoint})
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rrrt-analyzer",
			Namespace: cfg.Namespace,
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: &deadlineSeconds,
			BackoffLimit:          &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: "serving-ca",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "rrrt-serving-ca",
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "analyzer",
							Image: cfg.Image,
							Env:   env,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "serving-ca",
									MountPath: "/etc/pki/tls/serving-ca",
									ReadOnly:  true,
								},
							},
						},
					},
				},
			},
		},
	}
}
