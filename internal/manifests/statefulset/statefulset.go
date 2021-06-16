package statefulset

import (
	"context"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/status"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CompareFunc is the type for functions that compare two statefulsets.
// Return true if two statefulset are not not equal.
type CompareFunc func(current, desired *appsv1.StatefulSet) bool

// MutateFunc is the type for functions that mutate the current statefulset
// by applying the values from the desired statefulset.
type MutateFunc func(current, desired *appsv1.StatefulSet)

// Get returns the k8s statefulset for the given object key or an error.
func Get(ctx context.Context, c client.Client, key client.ObjectKey) (*appsv1.StatefulSet, error) {
	sts := New(key.Name, key.Namespace, nil, 1).Build()

	if err := c.Get(ctx, key, sts); err != nil {
		return nil, kverrors.Wrap(err, "failed to get statefulset",
			"name", sts.Name,
			"namespace", sts.Namespace,
			"cluster", sts.ClusterName,
		)
	}

	return sts, nil
}

// Create will create the given statefulset on the api server or return an error on failure
func Create(ctx context.Context, c client.Client, sts *appsv1.StatefulSet) (status.OperationResultType, error) {
	err := c.Create(ctx, sts)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, nil
	}

	return status.OperationResultNone, kverrors.Wrap(err, "failed to create statefulset",
		"name", sts.Name,
		"namespace", sts.Namespace,
		"cluster", sts.ClusterName,
	)
}

// Update will update an existing statefulset if compare func returns true or else leave it unchanged. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func Update(ctx context.Context, c client.Client, sts *appsv1.StatefulSet, cmp CompareFunc, mutate MutateFunc) (status.OperationResultType, error) {
	current := sts.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err := c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get statefulset",
			"name", current.Name,
			"namespace", current.Namespace,
			"cluster", current.ClusterName,
		)
	}

	if cmp(current, sts) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get statefulset", sts.Name)
				return err
			}

			mutate(current, sts)
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update statefulset", sts.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update statefulset",
				"name", sts.Name,
				"namespace", sts.Namespace,
				"cluster", sts.ClusterName,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}

// Delete attempts to delete a k8s statefulset if existing or returns an error.
func Delete(ctx context.Context, c client.Client, key client.ObjectKey) error {
	dpl := New(key.Name, key.Namespace, nil, 1).Build()

	if err := c.Delete(ctx, dpl, &client.DeleteOptions{}); err != nil {
		return kverrors.Wrap(err, "failed to delete statefulset",
			"name", dpl.Name,
			"namespace", dpl.Namespace,
			"cluster", dpl.ClusterName,
		)
	}

	return nil
}

// List returns a list of statefulsets that match the given selector.
func List(ctx context.Context, c client.Client, namespace string, selector map[string]string) ([]appsv1.StatefulSet, error) {
	list := &appsv1.StatefulSetList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(selector),
	}
	if err := c.List(ctx, list, opts...); err != nil {
		return nil, kverrors.Wrap(err, "failed to list statefulsets")
	}

	return list.Items, nil
}
