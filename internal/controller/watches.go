/*
Copyright 2025 Reactive Tech Limited.
"Reactive Tech Limited" is a company located in England, United Kingdom.
https://www.reactive-tech.io

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	core "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// appLabel is set by the StatefulSet templates on every Pod and, through the
// StatefulSet controller, on every PVC created from volumeClaimTemplates. It
// carries the Kubegres name, which lets objects that have no Kubegres owner
// reference still wake the right reconciler.
const appLabel = "app"

// mapKubegresChildByAppLabel maps a Pod or PVC to the Kubegres named by its
// app label. Objects without the label (unrelated workloads) map to nothing.
func mapKubegresChildByAppLabel(_ context.Context, obj client.Object) []reconcile.Request {
	name, ok := obj.GetLabels()[appLabel]
	if !ok || name == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: name}}}
}

// podChangePredicate admits the Pod events the reconciler acts on: voluntary
// disruption (DisruptionTarget), readiness and phase transitions, and
// deletion. Pod creation is already reflected in StatefulSet status.
func podChangePredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPod, okOld := e.ObjectOld.(*core.Pod)
			newPod, okNew := e.ObjectNew.(*core.Pod)
			if !okOld || !okNew {
				return false
			}
			if (oldPod.DeletionTimestamp == nil) != (newPod.DeletionTimestamp == nil) {
				return true
			}
			if oldPod.Status.Phase != newPod.Status.Phase {
				return true
			}
			return podCondition(oldPod, core.PodReady) != podCondition(newPod, core.PodReady) ||
				podCondition(oldPod, core.DisruptionTarget) != podCondition(newPod, core.DisruptionTarget)
		},
	}
}

func podCondition(pod *core.Pod, conditionType core.PodConditionType) core.PodCondition {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return core.PodCondition{Type: condition.Type, Status: condition.Status, Reason: condition.Reason}
		}
	}
	return core.PodCondition{Type: conditionType}
}

// pvcChangePredicate admits PVC events that matter to storage expansion:
// creation, deletion, and status changes (capacity or resize conditions).
func pvcChangePredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPvc, okOld := e.ObjectOld.(*core.PersistentVolumeClaim)
			newPvc, okNew := e.ObjectNew.(*core.PersistentVolumeClaim)
			if !okOld || !okNew {
				return false
			}
			return !apiequality.Semantic.DeepEqual(oldPvc.Status, newPvc.Status) ||
				!apiequality.Semantic.DeepEqual(oldPvc.Spec.Resources, newPvc.Spec.Resources)
		},
	}
}
