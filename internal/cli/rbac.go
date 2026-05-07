package cli

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	clusterRoleName    = "rrrt-analyzer"
	serviceAccountName = "rrrt-analyzer"
)

func createNamespace(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

func createServiceAccount(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}
	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	return err
}

func ensureClusterRole(ctx context.Context, clientset *kubernetes.Clientset) error {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRoleName},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"kubevirt.io"},
				Resources: []string{"virtualmachines", "virtualmachineinstances"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "statefulsets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get"},
			},
		},
	}

	_, err := clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = clientset.RbacV1().ClusterRoles().Update(ctx, cr, metav1.UpdateOptions{})
	}
	return err
}

func createClusterRoleBindings(ctx context.Context, clientset *kubernetes.Clientset, namespace string) ([]string, error) {
	analyzerCRB := fmt.Sprintf("rrrt-analyzer-%s", namespace)
	monitoringCRB := fmt.Sprintf("rrrt-monitoring-%s", namespace)

	crb1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: analyzerCRB},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	}

	crb2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: monitoringCRB},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view",
		},
	}

	if _, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb1, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("creating analyzer CRB: %w", err)
	}
	if _, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb2, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("creating monitoring CRB: %w", err)
	}

	return []string{analyzerCRB, monitoringCRB}, nil
}
