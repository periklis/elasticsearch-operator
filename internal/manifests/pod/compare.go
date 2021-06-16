package pod

import (
	"reflect"

	"github.com/openshift/elasticsearch-operator/internal/utils/comparators"

	corev1 "k8s.io/api/core/v1"
)

// ArePodTemplateSpecDifferent compares two corev1.PodTemplateSpec objects
// and returns true only if pod spec differ and tolerations are strictly the same
func ArePodTemplateSpecDifferent(lhs, rhs corev1.PodTemplateSpec) bool {
	return ArePodSpecDifferent(lhs.Spec, rhs.Spec, true)
}

// ArePodSpecDifferent compares two corev1.PodSpec objects and returns true
// only if they differ in any of the following:
// - Length of containers slice
// - Node selectors
// - Tolerations, if strict they need to be the same, non-strict for superset check
// - Containers: Name, Image, VolumeMounts, EnvVar, Args, Ports, ResourceRequirements
func ArePodSpecDifferent(lhs, rhs corev1.PodSpec, strictTolerations bool) bool {
	changed := false

	if len(lhs.Containers) != len(rhs.Containers) {
		changed = true
	}

	// check nodeselectors
	if !comparators.AreSelectorsSame(lhs.NodeSelector, rhs.NodeSelector) {
		changed = true
	}

	// strictTolerations are for when we compare from the deployments or statefulsets
	// if we are seeing if rolled out pods contain changes we don't want strictTolerations
	//   since k8s may add additional tolerations to pods
	if strictTolerations {
		// check tolerations
		if !comparators.AreTolerationsSame(lhs.Tolerations, rhs.Tolerations) {
			changed = true
		}
	} else {
		// check tolerations
		if !comparators.ContainsSameTolerations(lhs.Tolerations, rhs.Tolerations) {
			changed = true
		}
	}

	// check container fields
	for _, lContainer := range lhs.Containers {
		found := false

		for _, rContainer := range rhs.Containers {
			// Only compare the images of containers with the same name
			if lContainer.Name != rContainer.Name {
				continue
			}

			found = true

			// can't use reflect.DeepEqual here, due to k8s adding token mounts
			// check that rContainer is all found within lContainer and that they match by name
			if !comparators.ContainsSameVolumeMounts(lContainer.VolumeMounts, rContainer.VolumeMounts) {
				changed = true
			}

			if lContainer.Image != rContainer.Image {
				changed = true
			}

			if !comparators.EnvValueEqual(lContainer.Env, rContainer.Env) {
				changed = true
			}

			if !reflect.DeepEqual(lContainer.Args, rContainer.Args) {
				changed = true
			}

			if !reflect.DeepEqual(lContainer.Ports, rContainer.Ports) {
				changed = true
			}

			if !comparators.AreResourceRequementsSame(lContainer.Resources, rContainer.Resources) {
				changed = true
			}
		}

		if !found {
			changed = true
		}
	}
	return changed
}
