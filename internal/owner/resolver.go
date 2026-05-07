package owner

import (
	"context"
	"fmt"

	"github.com/kborup-redhat/rrrt/internal/types"
	corev1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Resolver struct {
	client client.Client
}

func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

func (r *Resolver) ResolveFromLabels(ctx context.Context, resourceLabels map[string]string, namespace string) (string, error) {
	if resourceLabels != nil {
		if owner, ok := resourceLabels[types.LabelOwner]; ok {
			return owner, nil
		}
	}

	ns := &corev1.Namespace{}
	if err := r.client.Get(ctx, k8stypes.NamespacedName{Name: namespace}, ns); err != nil {
		return "", fmt.Errorf("fetching namespace %s: %w", namespace, err)
	}

	if ns.Labels != nil {
		if owner, ok := ns.Labels[types.LabelOwner]; ok {
			return owner, nil
		}
	}

	return "", nil
}
