#!/bin/sh
set -eou pipefail

export OCP_VERSION=${OCP_VERSION:-4.7}
export LOGGING_VERSION=${LOGGING_VERSION:-5.2}
export LOGGING_ES_VERSION=${LOGGING_ES_VERSION:-6.8.1}
export LOGGING_KIBANA_VERSION=${LOGGING_KIBANA_VERSION:-6.8.1}
export LOGGING_ES_PROXY_VERSION=${LOGGING_ES_PROXY_VERSION:-1.0}
export LOGGING_CURATOR_VERSION=${LOGGING_CURATOR_VERSION:-5.8.1}
export LOGGING_IS=${LOGGING_IS:-openshift-logging}

#openshift images
export IMAGE_KUBE_RBAC_PROXY=${IMAGE_KUBE_RBAC_PROXY:-registry.ci.openshift.org/ocp/${OCP_VERSION}:kube-rbac-proxy}
export IMAGE_OAUTH_PROXY=${IMAGE_OAUTH_PROXY:-registry.ci.openshift.org/ocp/${OCP_VERSION}:oauth-proxy}

#logging images
IMAGE_ELASTICSEARCH_OPERATOR_REGISTRY=${IMAGE_ELASTICSEARCH_OPERATOR_REGISTRY:-quay.io/${LOGGING_IS}/elasticsearch-operator-registry:${LOGGING_VERSION}}
export IMAGE_ELASTICSEARCH_OPERATOR_REGISTRY=${IMAGE_ELASTICSEARCH_OPERATOR_REGISTRY:-$LOCAL_IMAGE_ELASTICSEARCH_OPERATOR_REGISTRY}

export IMAGE_ELASTICSEARCH_OPERATOR=${IMAGE_ELASTICSEARCH_OPERATOR:-quay.io/${LOGGING_IS}/elasticsearch-operator:${LOGGING_VERSION}}
export IMAGE_ELASTICSEARCH6=${IMAGE_ELASTICSEARCH6:-quay.io/${LOGGING_IS}/elasticsearch6:${LOGGING_ES_VERSION}}
export IMAGE_ELASTICSEARCH_PROXY=${IMAGE_ELASTICSEARCH_PROXY:-quay.io/${LOGGING_IS}/elasticsearch-proxy:${LOGGING_ES_PROXY_VERSION}}
export IMAGE_LOGGING_KIBANA6=${IMAGE_LOGGING_KIBANA6:-quay.io/${LOGGING_IS}/kibana6:${LOGGING_KIBANA_VERSION}}
export IMAGE_CURATOR5=${IMAGE_CURATOR5:-quay.io/${LOGGING_IS}/curator5:${LOGGING_CURATOR_VERSION}}

export ELASTICSEARCH_OPERATOR_NAMESPACE=${ELASTICSEARCH_OPERATOR_NAMESPACE:-openshift-operators-redhat}