package configmap

import (
	"context"
	"reflect"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/status"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CompareFunc is the type for functions that compare two configmaps.
// Return true if two configmaps are equal.
type CompareFunc func(current, desired *corev1.ConfigMap) bool

// MutateFunc is the type for functions that mutate the current configmap
// by applying the values from the desired configmap.
type MutateFunc func(current, desired *corev1.ConfigMap)

// Get returns the k8s configmap for the given object key or an error.
func Get(ctx context.Context, c client.Client, key client.ObjectKey) (*corev1.ConfigMap, error) {
	cm := New(key.Name, key.Namespace, nil, nil)

	if err := c.Get(ctx, key, cm); err != nil {
		return nil, kverrors.Wrap(err, "failed to get configmap",
			"name", cm.Name,
			"namespace", cm.Namespace,
		)
	}

	return cm, nil
}

// Create will create the given configmap on the api server or return an error on failure
func Create(ctx context.Context, c client.Client, cm *corev1.ConfigMap) (status.OperationResultType, error) {
	err := c.Create(ctx, cm)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, nil
	}

	return status.OperationResultNone, kverrors.Wrap(err, "failed to create configmap",
		"name", cm.Name,
		"namespace", cm.Namespace,
	)
}

// CreateOrUpdate attempts first to create the given configmap. If the
// configmap already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdate(ctx context.Context, c client.Client, cm *corev1.ConfigMap, cmp CompareFunc, mutate MutateFunc) (status.OperationResultType, error) {
	res, err := Create(ctx, c, cm)
	if res == status.OperationResultCreated {
		return res, nil
	}

	if err != nil {
		if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to create configmap",
				"name", cm.Name,
				"namespace", cm.Namespace,
			)
		}
	}

	current := cm.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get configmap",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !cmp(current, cm) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get configmap", cm.Name)
				return err
			}

			mutate(current, cm)
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update configmap", cm.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update configmap",
				"name", cm.Name,
				"namespace", cm.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}

// Delete attempts to delete a k8s configmap if existing or returns an error.
func Delete(ctx context.Context, c client.Client, key client.ObjectKey) error {
	cm := New(key.Name, key.Namespace, nil, nil)

	if err := c.Delete(ctx, cm, &client.DeleteOptions{}); err != nil {
		return kverrors.Wrap(err, "failed to delete configmap",
			"name", cm.Name,
			"namespace", cm.Namespace,
		)
	}

	return nil
}

// CompareDataOnly return only true if the configmaps have equal data sections only.
func CompareDataOnly(current, desired *corev1.ConfigMap) bool {
	return reflect.DeepEqual(current.Data, desired.Data)
}

// MutateDataOnly is a default mutate function implementation
// that copies only the data section from desired to current
// configmap.
func MutateDataOnly(current, desired *corev1.ConfigMap) {
	current.Data = desired.Data
}
