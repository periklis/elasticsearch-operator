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

// CreateOrUpdateRoleBinding attempts first to create the given rolebinding. If the
// rolebinding already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdateRoleBinding(ctx context.Context, c client.Client, rb *rbacv1.RoleBinding) (status.OperationResultType, error) {
	err := c.Create(ctx, rb)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create rolebinding",
			"name", rb.Name,
			"namespace", rb.Namespace,
		)
	}

	current := rb.DeepCopy()
	key := client.ObjectKey{Name: current.Name, Namespace: current.Namespace}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get rolebinding",
			"name", current.Name,
			"namespace", current.Namespace,
		)
	}

	if !equality.Semantic.DeepEqual(current, rb) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get rolebinding", rb.Name)
				return err
			}

			current.Subjects = rb.Subjects
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update rolebinding", rb.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update rolebinding",
				"name", rb.Name,
				"namespace", rb.Namespace,
			)
		}
		return status.OperationResultUpdated, nil
	}
	return status.OperationResultNone, nil
}

// Delete attempts to delete a k8s rolebinding if exists or returns eventually an error.
func DeleteRoleBinding(ctx context.Context, c client.Client, key client.ObjectKey) error {
	role := NewRoleBinding(key.Name, key.Namespace, "", nil)

	if err := c.Delete(ctx, role, &client.DeleteOptions{}); err != nil {
		return kverrors.Wrap(err, "failed to delete rolebinding",
			"name", role.Name,
			"namespace", role.Namespace,
		)
	}

	return nil
}
