package persistentvolume

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

// ComparePVCFunc is the type for functions that compare two persistentvolumeclaims.
// Return true if two persistentvolumeclaim are equal.
type ComparePVCFunc func(current, desired *corev1.PersistentVolumeClaim) bool

// MutatePVCFunc is the type for functions that mutate the current persistentvolumeclaim
// by applying the values from the desired persistentvolumeclaim.
type MutatePVCFunc func(current, desired *corev1.PersistentVolumeClaim)

// CreateOrUpdatePVC attempts first to create the given persistentvolumeclaim. If the
// persistentvolumeclaim already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdatePVC(ctx context.Context, c client.Client, pvc *corev1.PersistentVolumeClaim, cmp ComparePVCFunc, mutate MutatePVCFunc) (status.OperationResultType, error) {
	err := c.Create(ctx, pvc)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create persistentvolumeclaim",
			"name", pvc.Name,
			"namespace", pvc.Namespace,
		)
	}

	current := pvc.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get persistentvolumeclaim",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !cmp(current, pvc) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get persistentvolumeclaim", pvc.Name)
				return err
			}

			mutate(current, pvc)
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update persistentvolumeclaim", pvc.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update persistentvolumeclaim",
				"name", pvc.Name,
				"namespace", pvc.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}

// CompareLabelsOnly return only true if the pvcs are equal in labels only.
func CompareLabelsOnly(current, desired *corev1.PersistentVolumeClaim) bool {
	return reflect.DeepEqual(current.Labels, desired.Labels)
}

// MutateLabelsOnly is a default mutate function implementation
// that copies only the labels from desired to current persistentvolumeclaim.
func MutateLabelsOnly(current, desired *corev1.PersistentVolumeClaim) {
	current.Labels = desired.Labels
}

// List returns a list of pods that match the given selector.
func ListPVC(ctx context.Context, c client.Client, namespace string, selector map[string]string) ([]corev1.PersistentVolumeClaim, error) {
	list := &corev1.PersistentVolumeClaimList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(selector),
	}
	if err := c.List(ctx, list, opts...); err != nil {
		return nil, kverrors.Wrap(err, "failed to list persistentvolumeclaim",
			"namespace", namespace,
		)
	}

	return list.Items, nil
}
