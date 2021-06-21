package elasticsearch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/constants"
	"github.com/openshift/elasticsearch-operator/internal/manifests/secret"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdateSecretWithOwnerRef(secretName, namespace string, data map[string][]byte, client client.Client, ownerRef metav1.OwnerReference) error {
	s := secret.New(secretName, namespace, data)

	// add owner ref to secret
	s.OwnerReferences = append(s.OwnerReferences, ownerRef)

	res, err := secret.CreateOrUpdate(context.TODO(), client, s, secret.CompareDataEqual, secret.MutateDataOnly)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch secret",
			"owner_ref_name", ownerRef.Name,
		)
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch secret: %s", res),
		"secret_name", s.Name,
		"owner_ref_name", ownerRef.Name,
	)

	return nil
}

func CreateOrUpdateSecret(secretName, namespace string, data map[string][]byte, client client.Client) error {
	s := secret.New(secretName, namespace, data)

	res, err := secret.CreateOrUpdate(context.TODO(), client, s, secret.CompareDataEqual, secret.MutateDataOnly)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch secret")
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch secret: %s", res),
		"secret_name", s.Name,
	)

	return nil
}

func getSecretDataHash(secretName, namespace string, c client.Client) string {

	hash := ""

	key := client.ObjectKey{Name: secretName, Namespace: namespace}
	sec, err := secret.Get(context.TODO(), c, key)
	if err != nil {
		return hash
	}

	dataHashes := make(map[string][32]byte)

	for key, data := range sec.Data {
		dataHashes[key] = sha256.Sum256([]byte(data))
	}

	sortedKeys := sortDataHashKeys(dataHashes)

	for _, key := range sortedKeys {
		hash = fmt.Sprintf("%s%s", hash, dataHashes[key])
	}

	return hash
}

// hasRequiredSecrets will check that all secrets that we expect for EO to be able to communicate
// with the ES cluster it manages exist.
// It will return true if all required secrets/keys exist.
// Otherwise, it will return false and the message will be populated with what is missing.
func (er ElasticsearchRequest) hasRequiredSecrets() (bool, string) {
	message := ""
	hasRequired := true

	key := client.ObjectKey{Name: er.cluster.Name, Namespace: er.cluster.Namespace}
	sec, err := secret.Get(context.TODO(), er.client, key)

	// check that the secret is there
	if apierrors.IsNotFound(kverrors.Root(err)) {
		return false, fmt.Sprintf("Expected secret %q in namespace %q is missing", er.cluster.Name, er.cluster.Namespace)
	}

	var missingCerts []string
	var secretKeys []string

	for key, data := range sec.Data {
		// check that the fields aren't blank
		if string(data) == "" {
			missingCerts = append(missingCerts, key)
		}

		secretKeys = append(secretKeys, key)
	}

	// check the fields are there
	for _, key := range constants.ExpectedSecretKeys {
		if !sliceContainsString(secretKeys, key) {
			missingCerts = append(missingCerts, key)
		}
	}

	if len(missingCerts) > 0 {
		message = fmt.Sprintf("Secret %q fields are either missing or empty: [%s]", er.cluster.Name, strings.Join(missingCerts, ", "))
		hasRequired = false
	}

	return hasRequired, message
}
