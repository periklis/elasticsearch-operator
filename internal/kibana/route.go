package kibana

import (
	"context"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/configmap"
	"github.com/openshift/elasticsearch-operator/internal/manifests/rbac"
	"github.com/openshift/elasticsearch-operator/internal/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	consolev1 "github.com/openshift/api/console/v1"
	route "github.com/openshift/api/route/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const KibanaConsoleLinkName = "kibana-public-url"

func NewRouteWithCert(routeName, namespace, serviceName string, caCert []byte) *route.Route {
	r := NewRoute(routeName, namespace, serviceName)
	r.Spec.TLS.CACertificate = string(caCert)
	r.Spec.TLS.DestinationCACertificate = string(caCert)
	return r
}

// NewRoute stubs an instance of a Route
func NewRoute(routeName, namespace, serviceName string) *route.Route {
	return &route.Route{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Route",
			APIVersion: route.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: namespace,
			Labels: map[string]string{
				"component":     "support",
				"logging-infra": "support",
				"provider":      "openshift",
			},
		},
		Spec: route.RouteSpec{
			To: route.RouteTargetReference{
				Name: serviceName,
				Kind: "Service",
			},
			TLS: &route.TLSConfig{
				Termination:                   route.TLSTerminationReencrypt,
				InsecureEdgeTerminationPolicy: route.InsecureEdgeTerminationPolicyRedirect,
			},
		},
	}
}

// GetRouteURL retrieves the route URL from a given route and namespace
func (clusterRequest *KibanaRequest) GetRouteURL(routeName string) (string, error) {
	foundRoute := &route.Route{}

	if err := clusterRequest.Get(routeName, foundRoute); err != nil {
		if !apierrors.IsNotFound(kverrors.Root(err)) {
			log.Error(err, "Failed to check for kibana object")
		}
		return "", err
	}

	return fmt.Sprintf("%s%s", "https://", foundRoute.Spec.Host), nil
}

// RemoveRoute with given name and namespace
func (clusterRequest *KibanaRequest) RemoveRoute(routeName string) error {
	route := NewRoute(
		routeName,
		clusterRequest.cluster.Namespace,
		routeName,
	)

	err := clusterRequest.Delete(route)
	if err != nil && !apierrors.IsNotFound(kverrors.Root(err)) {
		return kverrors.Wrap(err, "failure deleting route",
			"route", routeName)
	}

	return nil
}

func (clusterRequest *KibanaRequest) CreateOrUpdateRoute(newRoute *route.Route) error {
	err := clusterRequest.Create(newRoute)
	if err == nil {
		return nil
	}

	errCtx := kverrors.NewContext(
		"cluster", clusterRequest.cluster.Name,
		"route", newRoute.Name,
	)

	if !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return errCtx.Wrap(err, "failure creating route for cluster")
	}

	// else -- try to update it if its a valid change (e.g. spec.tls)
	current := &route.Route{}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := clusterRequest.Get(newRoute.Name, current); err != nil {
			return errCtx.Wrap(err, "failed to get route")
		}

		if !reflect.DeepEqual(current.Spec.TLS, newRoute.Spec.TLS) {
			current.Spec.TLS = newRoute.Spec.TLS
			return clusterRequest.Update(current)
		}

		return nil
	})
	if err != nil {
		return errCtx.Wrap(err, "failed to update route")
	}
	return nil
}

func (clusterRequest *KibanaRequest) createOrUpdateKibanaRoute() error {
	cluster := clusterRequest.cluster

	var rt *route.Route
	fp := utils.GetWorkingDirFilePath("ca.crt")
	caCert, err := ioutil.ReadFile(fp)
	if err != nil {
		log.Info("could not read CA certificate for kibana route",
			"filePath", fp,
			"cause", err)
	}
	rt = NewRouteWithCert(
		"kibana",
		cluster.Namespace,
		"kibana",
		caCert,
	)

	utils.AddOwnerRefToObject(rt, getOwnerRef(cluster))

	err = clusterRequest.CreateOrUpdateRoute(rt)
	if err != nil && !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return kverrors.Wrap(err, "failed to update Kibana route for cluster",
			"cluster", cluster.Name)
	}

	return nil
}

func (clusterRequest *KibanaRequest) createOrUpdateKibanaConsoleLink() error {
	cluster := clusterRequest.cluster

	kibanaURL, err := clusterRequest.GetRouteURL("kibana")
	if err != nil {
		return kverrors.Wrap(err, "failed to get route URL for kibana")
	}

	cl := NewConsoleLink(KibanaConsoleLinkName, kibanaURL)
	utils.AddOwnerRefToObject(cl, getOwnerRef(cluster))

	if err := clusterRequest.createOrUpdateConsoleLink(cl); err != nil {
		return kverrors.Wrap(err, "failed to create or update kibana console link CR for cluster",
			"cluster", cluster.Name)
	}

	return nil
}

func (clusterRequest *KibanaRequest) createOrUpdateConsoleLink(desired *consolev1.ConsoleLink) error {
	linkName := desired.GetName()
	errCtx := kverrors.NewContext("cluster", clusterRequest.cluster.GetName(),
		"link_name", linkName)

	err := clusterRequest.Create(desired)
	if err != nil && !apierrors.IsAlreadyExists(kverrors.Root(err)) {
		return errCtx.Wrap(err, "failed to create Kibana link for cluster")
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &consolev1.ConsoleLink{}
		if err := clusterRequest.Get(linkName, current); err != nil {
			if apierrors.IsNotFound(kverrors.Root(err)) {
				return nil
			}
			return kverrors.Wrap(err, "failed to get Kibana console link", errCtx...)
		}

		ok := consoleLinksEqual(current, desired)
		if !ok {
			current.Spec = desired.Spec
			return clusterRequest.Update(current)
		}

		return nil
	})

	if err != nil {
		return kverrors.Wrap(err, "failed to update console link", errCtx...)
	}
	return nil
}

func (clusterRequest *KibanaRequest) createOrUpdateKibanaConsoleExternalLogLink() (err error) {
	cluster := clusterRequest.cluster

	errCtx := kverrors.NewContext("cluster", clusterRequest.cluster.Name,
		"namespace", clusterRequest.cluster.Namespace)

	kibanaURL, err := clusterRequest.GetRouteURL("kibana")
	if err != nil {
		return kverrors.Wrap(err, "failed to get route URL", errCtx...)
	}
	errCtx = append(errCtx, "kibana_url", kibanaURL)

	consoleExternalLogLink := NewConsoleExternalLogLink(
		"kibana",
		cluster.Namespace,
		"Show in Kibana",
		strings.Join([]string{
			kibanaURL,
			"/app/kibana#/discover?_g=(time:(from:now-1w,mode:relative,to:now))&_a=(columns:!(kubernetes.container_name,message),query:(query_string:(analyze_wildcard:!t,query:'",
			strings.Join([]string{
				"kubernetes.pod_name:\"${resourceName}\"",
				"kubernetes.namespace_name:\"${resourceNamespace}\"",
				"kubernetes.container_name.raw:\"${containerName}\"",
			}, " AND "),
			"')),sort:!('@timestamp',desc))",
		},
			""),
	)

	utils.AddOwnerRefToObject(consoleExternalLogLink, getOwnerRef(cluster))

	current := &consolev1.ConsoleExternalLogLink{}
	if err = clusterRequest.Get("kibana", current); err != nil {
		if !apierrors.IsNotFound(err) {
			return kverrors.Wrap(err, "failed to get consoleexternalloglink", errCtx...)
		}

		err = clusterRequest.Create(consoleExternalLogLink)
		if err != nil && !apierrors.IsAlreadyExists(kverrors.Root(err)) {
			return kverrors.Wrap(err, "failure creating Kibana console external log link", errCtx...)
		}

		return nil
	}

	// do a comparison to see if these are the same spec -- if not, delete and recreate
	if current.Spec.HrefTemplate != consoleExternalLogLink.Spec.HrefTemplate &&
		current.Spec.Text != consoleExternalLogLink.Spec.Text {

		if err = clusterRequest.RemoveConsoleExternalLogLink("kibana"); err != nil {
			return
		}

		err = clusterRequest.Create(consoleExternalLogLink)
		if err != nil && !apierrors.IsAlreadyExists(kverrors.Root(err)) {
			return kverrors.Wrap(err, "failure creating Kibana console external log link", errCtx...)
		}
	}

	return nil
}

func (clusterRequest *KibanaRequest) removeSharedConfigMapPre45x() error {
	cluster := clusterRequest.cluster

	errCtx := kverrors.NewContext("namespace", cluster.Namespace,
		"cluster", cluster.Name)

	sharedConfigKey := client.ObjectKey{Name: "sharing-config", Namespace: cluster.GetNamespace()}
	err := configmap.Delete(context.TODO(), clusterRequest.client, sharedConfigKey)
	if err != nil && !apierrors.IsNotFound(kverrors.Root(err)) {
		return kverrors.Wrap(err, "failed to delete Kibana route shared config",
			append(errCtx, "configmap", sharedConfigKey.Name)...)
	}

	sharedRoleKey := client.ObjectKey{Name: "sharing-config-reader", Namespace: cluster.Namespace}
	err = rbac.DeleteRole(context.TODO(), clusterRequest.client, sharedRoleKey)
	if err != nil && !apierrors.IsNotFound(kverrors.Root(err)) {
		return kverrors.Wrap(err, "failed to delete Kibana route shared config role",
			append(errCtx, "role_name", sharedRoleKey.Name)...)
	}

	sharedRoleBindingKey := client.ObjectKey{Name: "openshift-logging-sharing-config-reader-binding", Namespace: cluster.Namespace}
	err = rbac.DeleteRoleBinding(context.TODO(), clusterRequest.client, sharedRoleBindingKey)
	if err != nil && !apierrors.IsNotFound(kverrors.Root(err)) {
		return kverrors.Wrap(err, "failed to delete Kibana route shared config rolebinding",
			append(errCtx, "role_binding_name", sharedRoleBindingKey.Name)...)
	}

	return nil
}
