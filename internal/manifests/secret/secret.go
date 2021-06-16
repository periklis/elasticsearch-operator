package secret

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

// CompareFunc is the type for functions that compare two secrets.
// Return true if two secrets are equal.
type CompareFunc func(current, desired *corev1.Secret) bool

// MutateFunc is the type for functions that mutate the current secret
// by applying the values from the desired secret.
type MutateFunc func(current, desired *corev1.Secret)

// Get returns the k8s secret for the given object key or an error.
func Get(ctx context.Context, c client.Client, key client.ObjectKey) (*corev1.Secret, error) {
	s := New(key.Name, key.Namespace, map[string][]byte{})

	if err := c.Get(ctx, key, s); err != nil {
		return nil, kverrors.Wrap(err, "failed to get secret",
			"name", s.Name,
			"namespace", s.Namespace,
		)
	}

	return s, nil
}

// CreateOrUpdate attempts first to create the given secret. If the
// secret already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdate(ctx context.Context, c client.Client, svc *corev1.Secret, cmp CompareFunc, mutate MutateFunc) (status.OperationResultType, error) {
	err := c.Create(ctx, svc)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create secret",
			"name", svc.Name,
			"namespace", svc.Namespace,
		)
	}

	current := svc.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get secret",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !cmp(current, svc) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get secret", svc.Name)
				return err
			}

			mutate(current, svc)
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update secret", svc.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update secret",
				"name", svc.Name,
				"namespace", svc.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}

// CompareDataEqual returns true only if the data of current and desird are exactly same.
func CompareDataEqual(current, desired *corev1.Secret) bool {
	if len(current.Data) != len(desired.Data) {
		return false
	}

	for lKey, lVal := range current.Data {
		keyFound := false
		for rKey, rVal := range desired.Data {
			if lKey == rKey {
				keyFound = true

				if !reflect.DeepEqual(lVal, rVal) {
					return false
				}
			}
		}

		if !keyFound {
			return false
		}
	}

	return true
}

// MutateDataOnly is a default mutation function for services
// that copies only the data field from desired to current.
func MutateDataOnly(current, desired *corev1.Secret) {
	current.Data = desired.Data
}
