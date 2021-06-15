package service

import (
	"context"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/status"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CompareFunc is the type for functions that compare two services.
// Return true if two services are equal.
type CompareFunc func(current, desired *corev1.Service) bool

// MutateFunc is the type for functions that mutate the current service
// by applying the values from the desired service.
type MutateFunc func(current, desired *corev1.Service)

// CreateOrUpdate attempts first to create the given service. If the
// service already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See OperationResultType) and eventually an error.
func CreateOrUpdate(ctx context.Context, c client.Client, svc *corev1.Service, cmp CompareFunc, mutate MutateFunc) (status.OperationResultType, error) {
	err := c.Create(ctx, svc)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create service",
			"name", svc.Name,
			"namespace", svc.Namespace,
		)
	}

	current := svc.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get service",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !cmp(current, svc) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get service", svc.Name)
				return err
			}

			mutate(current, svc)
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update service", svc.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update service",
				"name", svc.Name,
				"namespace", svc.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}

// Compare return only true if the service are equal
func Compare(current, desired *corev1.Service) bool {
	return equality.Semantic.DeepEqual(current, desired)
}

// Mutate is a default mutation function for services
// that copies only mutable fields from desired to current.
func Mutate(current, desired *corev1.Service) {
	current.Labels = desired.Labels
	current.Annotations = desired.Annotations
	current.Spec.Ports = desired.Spec.Ports
	current.Spec.Selector = desired.Spec.Selector
	current.Spec.PublishNotReadyAddresses = desired.Spec.PublishNotReadyAddresses
}
