package elasticsearch

import (
	"context"
	"fmt"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/serviceaccount"
)

// CreateOrUpdateServiceAccount ensures the existence of the serviceaccount for Elasticsearch cluster
func (er *ElasticsearchRequest) CreateOrUpdateServiceAccount() (err error) {
	dpl := er.cluster

	sa := serviceaccount.New(dpl.Name, dpl.Namespace, map[string]string{})
	er.cluster.AddOwnerRefTo(sa)

	res, err := serviceaccount.CreateOrUpdate(context.TODO(), er.client, sa)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch serviceaccount",
			"cluster", dpl.Name,
			"namespace", dpl.Namespace,
		)
	}

	log.Info(fmt.Sprintf("Successfully reconciled elasticsearch serviceaccount: %s", res),
		"service_account_name", sa.Name,
		"cluster", dpl.Name,
		"namespace", dpl.Namespace,
	)

	return nil
}
