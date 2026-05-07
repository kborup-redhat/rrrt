---
title: "Chapter 5: Owner Resolver"
order: 5
---

# Chapter 5: Owner Resolver

## Introduction

Knowing that a VM should be downsized is useful, but knowing *who to tell* is essential. The Owner Resolver handles attribution — it figures out who is responsible for each resource so the report can list an owner name next to every recommendation. Think of it as a phone directory lookup: given a resource, find the person who should act on the recommendation.

## How It Works

The Owner Resolver is a small, focused component in `internal/owner/resolver.go`:

```go
type Resolver struct {
    client client.Client
}

func NewResolver(c client.Client) *Resolver {
    return &Resolver{client: c}
}
```

It has a single method that implements a two-level lookup:

```go
func (r *Resolver) ResolveFromLabels(ctx context.Context,
    resourceLabels map[string]string, namespace string) (string, error) {

    // Level 1: Check the resource's own labels
    if resourceLabels != nil {
        if owner, ok := resourceLabels[types.LabelOwner]; ok {
            return owner, nil
        }
    }

    // Level 2: Fall back to the namespace's labels
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
```

### The Lookup Chain

1. **Resource-level**: Check if the VM, Deployment, or StatefulSet has the label `rightsizing.redhatconsulting.io/owner` set directly
2. **Namespace-level**: If not found on the resource, check the namespace for the same label

This two-level approach is practical because:
- Teams that own entire namespaces can set the owner label once on the namespace
- Individual resources that have different owners (e.g., a shared namespace) can override with their own label

If neither level has the label, an empty string is returned — the resource simply shows no owner in the report. This is a deliberate choice: missing ownership is informational, not an error.

### How It's Used

The Collector calls the resolver for every resource it analyzes:

```go
ownerStr, _ := c.owner.ResolveFromLabels(ctx, vm.GetLabels(), ns)
```

The error is intentionally discarded with `_` — a namespace lookup failure shouldn't prevent the resource from being analyzed. The resource simply appears with no owner attribution.

### Setting Owner Labels

To use owner attribution, operators label their resources or namespaces:

```bash
# Label a namespace (applies to all resources in it)
oc label namespace my-app rightsizing.redhatconsulting.io/owner="platform-team@example.com"

# Label a specific VM (overrides namespace-level)
oc label vm my-database -n my-app rightsizing.redhatconsulting.io/owner="dba-team@example.com"
```

## Relationships

- Called by the **Collector** for each VM, Deployment, and StatefulSet
- Uses the well-known label key defined in the **Types** package (`types.LabelOwner`)
- The resolved owner string is stored in `ResourceAnalysis.Owner` and displayed by the **PDF Generator** in both tables and detail cards

## Key Takeaways

- Two-level lookup: resource labels first, then namespace labels as a fallback
- Missing ownership returns an empty string — it's informational, not an error
- The label key is `rightsizing.redhatconsulting.io/owner` (defined in the `types` package)
- Namespace-level labels are a convenient way to set ownership for all resources in a namespace at once

Next, we'll look at how the PDF generator turns all this data into a professional report.
