package console

import (
	"context"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/status"

	consolev1 "github.com/openshift/api/console/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CompareConsoleLinkFunc is the type for functions that compare two consolelinks.
// Return true if two consolelinks are equal.
type CompareConsoleLinkFunc func(current, desired *consolev1.ConsoleLink) bool

// MutateConsoleLinkFunc is the type for functions that mutate the current consolelink
// by applying the values from the desired consolelink.
type MutateConsoleLinkFunc func(current, desired *consolev1.ConsoleLink)

// CreateOrUpdateConsoleLink attempts first to create the given consolelink. If the
// consolelink already exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns the operation result (See status.OperationResultType) and eventually an error.
func CreateOrUpdateConsoleLink(ctx context.Context, c client.Client, cl *consolev1.ConsoleLink, cmp CompareConsoleLinkFunc, mutate MutateConsoleLinkFunc) (status.OperationResultType, error) {
	err := c.Create(ctx, cl)
	if err == nil {
		return status.OperationResultCreated, nil
	}

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to create consolelink",
			"name", cl.Name,
		)
	}

	current := cl.DeepCopy()
	key := client.ObjectKey{Name: current.Name}
	err = c.Get(ctx, key, current)
	if err != nil {
		return status.OperationResultNone, kverrors.Wrap(err, "failed to get consolelink",
			"name", current.Name,
		)
	}

	if !cmp(current, cl) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				log.Error(err, "failed to get consolelink", cl.Name)
				return err
			}

			mutate(current, cl)
			if err := c.Update(ctx, current); err != nil {
				log.Error(err, "failed to update consolelink", cl.Name)
				return err
			}
			return nil
		})
		if err != nil {
			return status.OperationResultNone, kverrors.Wrap(err, "failed to update consolelink",
				"name", cl.Name,
			)
		}
		return status.OperationResultUpdated, nil
	}

	return status.OperationResultNone, nil
}

// CompareConsoleLinks returns true all of the following are equal:
// - location
// - link text
// - link href
// - application menu section
func CompareConsoleLinks(current, desired *consolev1.ConsoleLink) bool {
	if current.Spec.Location != desired.Spec.Location {
		return false
	}

	if current.Spec.Link.Text != desired.Spec.Link.Text {
		return false
	}

	if current.Spec.Link.Href != desired.Spec.Link.Href {
		return false
	}

	if current.Spec.ApplicationMenu.Section != desired.Spec.ApplicationMenu.Section {
		return false
	}

	return true
}

// MutateSpecOnly is a default mutate implementation that copies
// only the spec from desired to current consolelink.
func MutateConsoleLinkSpecOnly(current, desired *consolev1.ConsoleLink) {
	current.Spec = desired.Spec
}
