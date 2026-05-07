package owner_test

import (
	"context"
	"testing"

	"github.com/kborup-redhat/rrrt/internal/owner"
	"github.com/kborup-redhat/rrrt/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveFromResourceLabels(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	r := owner.NewResolver(c)

	labels := map[string]string{types.LabelOwner: "alice@example.com"}
	result, err := r.ResolveFromLabels(context.Background(), labels, "default")
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", result)
}

func TestResolveFromNamespaceFallback(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "prod",
			Labels: map[string]string{types.LabelOwner: "team-infra@example.com"},
		},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	r := owner.NewResolver(c)
	result, err := r.ResolveFromLabels(context.Background(), nil, "prod")
	require.NoError(t, err)
	assert.Equal(t, "team-infra@example.com", result)
}

func TestResolveNoOwner(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

	r := owner.NewResolver(c)
	result, err := r.ResolveFromLabels(context.Background(), nil, "default")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}
