package servicemonitor

import (
	"context"
	"reflect"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/status"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CompareFunc is the type for functions that compare two servicemonitors.
// Return true if two service are equal.
type CompareFunc func(current, desired *monitoringv1.ServiceMonitor) bool

// MutateFunc is the type for functions that mutate the current servicemonitor
// by applying the values from the desired service.
type MutateFunc func(current, desired *monitoringv1.ServiceMonitor)

// CreateOrUpdate attempts first to create the given servicemonitor. If the
// servicemonitor already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See OperationResultType) and eventually an error.
func CreateOrUpdate(ctx context.Context, c client.Client, sm *monitoringv1.ServiceMonitor, cmp CompareFunc, mutate MutateFunc) (status.OperationResultType, error) {
	err := c.Create(ctx, sm)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create servicemonitor",
			"name", sm.Name,
			"namespace", sm.Namespace,
		)
	}

	current := sm.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get servicemonitor",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !cmp(current, sm) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get servicemonitor", sm.Name)
				return err
			}

			mutate(current, sm)
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update servicemonitor", sm.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update servicemonitor",
				"name", sm.Name,
				"namespace", sm.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}

// Compare return only true if the service monitors are equal
func Compare(current, desired *monitoringv1.ServiceMonitor) bool {
	// TODO Consider switching this to equality.Semantic.DeepEqual
	//      after investigating if it is supported on prometheus-operator
	//      types.
	return reflect.DeepEqual(current, desired)
}

// Mutate is a default mutation function for servicemonitors
// that copies only mutable fields from desired to current.
func Mutate(current, desired *monitoringv1.ServiceMonitor) {
	current.Labels = desired.Labels
	current.Spec.JobLabel = desired.Spec.JobLabel
	current.Spec.Endpoints = desired.Spec.Endpoints
}
