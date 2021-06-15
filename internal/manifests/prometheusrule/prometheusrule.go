package prometheusrule

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

// CreateOrUpdate attempts first to create the given prometheusrule. If the
// prometheusrule already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdate(ctx context.Context, c client.Client, pr *monitoringv1.PrometheusRule) (status.OperationResultType, error) {
	err := c.Create(ctx, pr)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create prometheusrule",
			"name", pr.Name,
			"namespace", pr.Namespace,
		)
	}

	current := pr.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get prometheusrule",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	// TODO Consider switching this to equality.Semantic.DeepEqual
	//      after investigating if it is supported on prometheus-operator
	//      types.
	if !reflect.DeepEqual(current, pr) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get prometheusrule", pr.Name)
				return err
			}

			current.Spec = pr.Spec
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update prometheusrule", pr.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update prometheusrule",
				"name", pr.Name,
				"namespace", pr.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}
