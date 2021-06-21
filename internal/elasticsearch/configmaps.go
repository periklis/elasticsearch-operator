package elasticsearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"html/template"
	"io"
	"runtime"
	"strconv"

	"github.com/ViaQ/logerr/kverrors"
	"github.com/ViaQ/logerr/log"
	"github.com/openshift/elasticsearch-operator/internal/manifests/configmap"
	"github.com/openshift/elasticsearch-operator/internal/manifests/status"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	esConfig            = "elasticsearch.yml"
	log4jConfig         = "log4j2.properties"
	indexSettingsConfig = "index_settings"
)

// esYmlStruct is used to render esYmlTmpl to a proper elasticsearch.yml format
type esYmlStruct struct {
	KibanaIndexMode      string
	EsUnicastHost        string
	NodeQuorum           string
	RecoverExpectedNodes string
	SystemCallFilter     string
}

type log4j2PropertiesStruct struct {
	RootLogger       string
	LogLevel         string
	SecurityLogLevel string
}

type indexSettingsStruct struct {
	PrimaryShards string
	ReplicaShards string
}

// CreateOrUpdateConfigMaps ensures the existence of ConfigMaps with Elasticsearch configuration
func (er *ElasticsearchRequest) CreateOrUpdateConfigMaps() (err error) {
	dpl := er.cluster

	kibanaIndexMode, err := kibanaIndexMode("")
	if err != nil {
		return err
	}
	dataNodeCount := int(GetDataCount(dpl))
	masterNodeCount := int(getMasterCount(dpl))

	logConfig := getLogConfig(dpl.GetAnnotations())

	cm := newConfigMap(
		dpl.Name,
		dpl.Namespace,
		dpl.Labels,
		kibanaIndexMode,
		esUnicastHost(dpl.Name, dpl.Namespace),
		strconv.Itoa(masterNodeCount/2+1),
		strconv.Itoa(dataNodeCount),
		strconv.Itoa(CalculatePrimaryCount(dpl)),
		strconv.Itoa(CalculateReplicaCount(dpl)),
		strconv.FormatBool(runtime.GOARCH == "amd64"),
		logConfig,
	)

	dpl.AddOwnerRefTo(cm)

	res, err := configmap.CreateOrUpdate(context.TODO(), er.client, cm, configMapContentEqual, configmap.MutateDataOnly)
	if err != nil {
		return kverrors.Wrap(err, "failed to create or upadte elasticsearch configmap",
			"cluster", er.cluster.Name,
			"namespace", er.cluster.Namespace,
		)
	}

	if res == status.OperationResultUpdated {
		// Cluster settings has changed, make sure it doesnt go unnoticed
		if err := updateConditionWithRetry(dpl, v1.ConditionTrue, updateUpdatingSettingsCondition, er.client); err != nil {
			return err
		}
	} else {
		if err := updateConditionWithRetry(dpl, v1.ConditionFalse, updateUpdatingSettingsCondition, er.client); err != nil {
			return err
		}
	}

	log.V(1).Info(fmt.Sprintf("Successfully reconciled elasticsearch config map: %s", res),
		"configmap_name", cm.Name,
		"cluster", er.cluster.Name,
		"namespace", er.cluster.Namespace,
	)

	return nil
}

func renderData(kibanaIndexMode, esUnicastHost, nodeQuorum, recoverExpectedNodes, primaryShardsCount, replicaShardsCount, systemCallFilter string, logConfig LogConfig) (map[string]string, error) {
	data := map[string]string{}
	buf := &bytes.Buffer{}
	if err := renderEsYml(buf, kibanaIndexMode, esUnicastHost, nodeQuorum, recoverExpectedNodes, systemCallFilter); err != nil {
		return data, err
	}
	data[esConfig] = buf.String()

	buf = &bytes.Buffer{}
	if err := renderLog4j2Properties(buf, logConfig); err != nil {
		return data, err
	}
	data[log4jConfig] = buf.String()

	buf = &bytes.Buffer{}
	if err := renderIndexSettings(buf, primaryShardsCount, replicaShardsCount); err != nil {
		return data, err
	}
	data[indexSettingsConfig] = buf.String()

	return data, nil
}

// newConfigMap returns a v1.ConfigMap object
func newConfigMap(configMapName, namespace string, labels map[string]string,
	kibanaIndexMode, esUnicastHost, nodeQuorum, recoverExpectedNodes, primaryShardsCount, replicaShardsCount, systemCallFilter string, logConfig LogConfig) *v1.ConfigMap {
	data, err := renderData(kibanaIndexMode, esUnicastHost, nodeQuorum, recoverExpectedNodes, primaryShardsCount, replicaShardsCount, systemCallFilter, logConfig)
	if err != nil {
		return nil
	}

	return configmap.New(configMapName, namespace, labels, data)
}

func configMapContentEqual(old, new *v1.ConfigMap) bool {
	oldEsConfigSum := sha256.Sum256([]byte(old.Data[esConfig]))
	newEsConfigSum := sha256.Sum256([]byte(new.Data[esConfig]))

	if oldEsConfigSum != newEsConfigSum {
		return false
	}

	oldLog4jConfig := sha256.Sum256([]byte(old.Data[log4jConfig]))
	newLog4jConfig := sha256.Sum256([]byte(new.Data[log4jConfig]))

	if oldLog4jConfig != newLog4jConfig {
		return false
	}

	oldIndexSettingsConfig := sha256.Sum256([]byte(old.Data[indexSettingsConfig]))
	newIndexSettingsConfig := sha256.Sum256([]byte(new.Data[indexSettingsConfig]))

	if oldIndexSettingsConfig != newIndexSettingsConfig {
		return false
	}

	return true
}

func renderEsYml(w io.Writer, kibanaIndexMode, esUnicastHost, nodeQuorum, recoverExpectedNodes, systemCallFilter string) error {
	t := template.New("elasticsearch.yml")
	config := esYmlTmpl
	t, err := t.Parse(config)
	if err != nil {
		return err
	}
	esy := esYmlStruct{
		KibanaIndexMode:      kibanaIndexMode,
		EsUnicastHost:        esUnicastHost,
		NodeQuorum:           nodeQuorum,
		RecoverExpectedNodes: recoverExpectedNodes,
		SystemCallFilter:     systemCallFilter,
	}

	return t.Execute(w, esy)
}

func renderLog4j2Properties(w io.Writer, logConfig LogConfig) error {
	t := template.New("log4j2.properties")
	t, err := t.Parse(log4j2PropertiesTmpl)
	if err != nil {
		return err
	}

	log4jProp := log4j2PropertiesStruct{
		RootLogger:       logConfig.ServerAppender,
		LogLevel:         logConfig.ServerLoglevel,
		SecurityLogLevel: logConfig.LogLevel,
	}

	return t.Execute(w, log4jProp)
}

func renderIndexSettings(w io.Writer, primaryShardsCount, replicaShardsCount string) error {
	t := template.New("index_settings")
	t, err := t.Parse(indexSettingsTmpl)
	if err != nil {
		return err
	}

	indexSettings := indexSettingsStruct{
		PrimaryShards: primaryShardsCount,
		ReplicaShards: replicaShardsCount,
	}

	return t.Execute(w, indexSettings)
}

func getConfigmapDataHash(configmapName, namespace string, c client.Client) string {
	hash := ""

	key := client.ObjectKey{Name: configmapName, Namespace: namespace}
	cm, err := configmap.Get(context.TODO(), c, key)
	if err != nil {
		// TODO Fix missing error handling
	}

	dataHashes := make(map[string][32]byte)

	for key, data := range cm.Data {
		if key != "index_settings" {
			dataHashes[key] = sha256.Sum256([]byte(data))
		}
	}

	sortedKeys := sortDataHashKeys(dataHashes)

	for _, key := range sortedKeys {
		hash = fmt.Sprintf("%s%s", hash, dataHashes[key])
	}

	return hash
}
