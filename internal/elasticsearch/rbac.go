package elasticsearch

import (
	"context"
	"fmt"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/openshift/elasticsearch-operator/apis/logging/v1"
	"github.com/openshift/elasticsearch-operator/internal/manifests/rbac"
	rbacv1 "k8s.io/api/rbac/v1"
)

func (er *ElasticsearchRequest) CreateOrUpdateRBAC() error {
	dpl := er.cluster

	// elasticsearch RBAC
	elasticsearchRole := rbac.NewClusterRole(
		"elasticsearch-metrics",
		rbac.NewPolicyRules(
			rbac.NewPolicyRule(
				[]string{""},
				[]string{"pods", "services", "endpoints"},
				[]string{},
				[]string{"list", "watch"},
				[]string{},
			),
			rbac.NewPolicyRule(
				[]string{},
				[]string{},
				[]string{},
				[]string{"get"},
				[]string{"/metrics"},
			),
		),
	)

	res, err := rbac.CreateOrUpdateClusterRole(context.TODO(), er.client, elasticsearchRole)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch clusterrole",
			"cluster", dpl.Name,
			"namespace", dpl.Namespace,
		)
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch clusterrole: %s", res),
		"cluster_role_name", elasticsearchRole.Name,
		"cluster", dpl.Name,
		"namespace", dpl.Namespace,
	)

	subject := rbac.NewSubject(
		"ServiceAccount",
		"prometheus-k8s",
		"openshift-monitoring",
	)
	subject.APIGroup = ""

	elasticsearchRoleBinding := rbac.NewClusterRoleBinding(
		"elasticsearch-metrics",
		"elasticsearch-metrics",
		rbac.NewSubjects(subject),
	)

	res, err = rbac.CreateOrUpdateClusterRoleBinding(context.TODO(), er.client, elasticsearchRoleBinding)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch clusterrolebinding",
			"cluster_role_binding_name", elasticsearchRoleBinding.Name,
		)
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch clusterrolebinding: %s", res),
		"cluster_role_binding_name",
		elasticsearchRoleBinding.Name,
		"cluster", dpl.Name,
		"namespace", dpl.Namespace,
	)

	// proxy RBAC
	proxyRole := rbac.NewClusterRole(
		"elasticsearch-proxy",
		rbac.NewPolicyRules(
			rbac.NewPolicyRule(
				[]string{"authentication.k8s.io"},
				[]string{"tokenreviews"},
				[]string{},
				[]string{"create"},
				[]string{},
			),
			rbac.NewPolicyRule(
				[]string{"authorization.k8s.io"},
				[]string{"subjectaccessreviews"},
				[]string{},
				[]string{"create"},
				[]string{},
			),
		),
	)

	res, err = rbac.CreateOrUpdateClusterRole(context.TODO(), er.client, proxyRole)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch proxy clusterrole",
			"cluster", dpl.Name,
			"namespace", dpl.Namespace,
		)
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch proxy clusterrole: %s", res),
		"cluster_role_name", proxyRole.Name,
		"cluster", dpl.Name,
		"namespace", dpl.Namespace,
	)

	// Cluster role elasticsearch-proxy has to contain subjects for all ES instances
	esList := &v1.ElasticsearchList{}
	err = er.client.List(context.TODO(), esList)
	if err != nil {
		return err
	}

	subjects := []rbacv1.Subject{}
	for _, es := range esList.Items {
		subject = rbac.NewSubject(
			"ServiceAccount",
			es.Name,
			es.Namespace,
		)
		subject.APIGroup = ""
		subjects = append(subjects, subject)
	}

	proxyRoleBinding := rbac.NewClusterRoleBinding(
		"elasticsearch-proxy",
		"elasticsearch-proxy",
		subjects,
	)

	res, err = rbac.CreateOrUpdateClusterRoleBinding(context.TODO(), er.client, proxyRoleBinding)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch proxy clusterrolebinding",
			"cluster_role_binding_name", proxyRoleBinding.Name,
		)
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch proxy clusterrolebinding: %s", res),
		"cluster_role_binding_name",
		proxyRoleBinding.Name,
		"cluster", dpl.Name,
		"namespace", dpl.Namespace,
	)

	return reconcileIndexManagmentRbac(dpl, er.client)
}

// TODO Move this to internal/indexmanagement
func reconcileIndexManagmentRbac(cluster *v1.Elasticsearch, client client.Client) error {
	role := rbac.NewRole(
		"elasticsearch-index-management",
		cluster.Namespace,
		rbac.NewPolicyRules(
			rbac.NewPolicyRule(
				[]string{"elasticsearch.openshift.io"},
				[]string{"indices"},
				[]string{},
				[]string{"*"},
				[]string{},
			),
		),
	)

	cluster.AddOwnerRefTo(role)

	res, err := rbac.CreateOrUpdateRole(context.TODO(), client, role)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch index management role",
			"cluster", cluster.Name,
			"namespace", cluster.Namespace,
		)
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch index management role: %s", res),
		"role_name", role.Name,
		"cluster", cluster.Name,
		"namespace", cluster.Namespace,
	)

	subject := rbac.NewSubject(
		"ServiceAccount",
		cluster.Name,
		cluster.Namespace,
	)
	subject.APIGroup = ""
	roleBinding := rbac.NewRoleBinding(
		role.Name,
		role.Namespace,
		role.Name,
		rbac.NewSubjects(subject),
	)
	cluster.AddOwnerRefTo(roleBinding)

	res, err = rbac.CreateOrUpdateRoleBinding(context.TODO(), client, roleBinding)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or update elasticsearch index management rolebinding",
			"cluster", cluster.Name,
			"namespace", cluster.Namespace,
		)
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch index management rolebinding: %s", res),
		"role_binding_name", roleBinding.Name,
		"cluster", cluster.Name,
		"namespace", cluster.Namespace,
	)

	return nil
}
