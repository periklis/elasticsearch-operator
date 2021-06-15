package rbac

import (
	"context"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/status"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrUpdateRole attempts first to create the given role. If the
// role already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdateRole(ctx context.Context, c client.Client, r *rbacv1.Role) (status.OperationResultType, error) {
	err := c.Create(ctx, r)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create role",
			"name", r.Name,
			"namespace", r.Namespace,
		)
	}

	current := r.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get role",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !equality.Semantic.DeepEqual(current, r) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get role", r.Name)
				return err
			}

			current.Rules = r.Rules
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update role", r.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update role",
				"name", r.Name,
				"namespace", r.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}
	return status.OperationResultNone, nil
}

// Delete attempts to delete a k8s role if exists or returns eventually an error.
func DeleteRole(ctx context.Context, c client.Client, key client.ObjectKey) error {
	role := NewRole(key.Name, key.Namespace, nil)

	if err := c.Delete(ctx, role, &client.DeleteOptions{}); err != nil {
		return kverrors.Wrap(err, "failed to delete role",
			"name", role.Name,
			"namespace", role.Namespace,
		)
	}

	return nil
}
