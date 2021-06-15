package serviceaccount

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

// CreateOrUpdate attempts first to create the given serviceaccount. If the
// serviceaccount already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdate(ctx context.Context, c client.Client, sa *corev1.ServiceAccount) (status.OperationResultType, error) {
	err := c.Create(ctx, sa)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create serviceaccount",
			"name", sa.Name,
			"namespace", sa.Namespace,
		)
	}

	current := sa.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get serviceaccount",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !equality.Semantic.DeepEqual(current, sa) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get serviceaccount", sa.Name)
				return err
			}

			current = sa
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update serviceaccount", sa.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update serviceaccount",
				"name", sa.Name,
				"namespace", sa.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}
