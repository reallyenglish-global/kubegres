/*
Copyright 2023 Reactive Tech Limited.
"Reactive Tech Limited" is a company located in England, United Kingdom.
https://www.reactive-tech.io

Lead Developer: Alex Arica

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

package resources_count_spec

import (
	core "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reactive-tech.io/kubegres/internal/controller/spec/template"
	"reactive-tech.io/kubegres/internal/controller/states"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServicesCountSpecEnforcer struct {
	kubegresContext  ctx.KubegresContext
	resourcesStates  states.ResourcesStates
	resourcesCreator template.ResourcesCreatorFromTemplate
}

func CreateServicesCountSpecEnforcer(kubegresContext ctx.KubegresContext,
	resourcesStates states.ResourcesStates,
	resourcesCreator template.ResourcesCreatorFromTemplate) ServicesCountSpecEnforcer {

	return ServicesCountSpecEnforcer{
		kubegresContext:  kubegresContext,
		resourcesStates:  resourcesStates,
		resourcesCreator: resourcesCreator,
	}
}

func (r *ServicesCountSpecEnforcer) EnforceSpec() error {

	if err := r.ensurePodDisruptionBudget(); err != nil {
		return err
	}

	if !r.isPrimaryServiceDeployed() && r.isPrimaryDbReady() {
		err := r.deployPrimaryService()
		if err != nil {
			return err
		}
	}

	if !r.isReplicaServiceDeployed() && r.isThereReadyReplica() {
		err := r.deployReplicaService()
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *ServicesCountSpecEnforcer) ensurePodDisruptionBudget() error {
	kubegres := r.kubegresContext.Kubegres
	name := kubegres.Name
	current := &policy.PodDisruptionBudget{}
	err := r.kubegresContext.Client.Get(r.kubegresContext.Ctx, client.ObjectKey{Name: name, Namespace: kubegres.Namespace}, current)
	enabled := kubegres.Spec.PodDisruptionBudget.Enabled || kubegres.Spec.Failover.OnPrimaryPodDrain
	if apierrors.IsNotFound(err) && !enabled {
		return nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if !enabled {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return r.kubegresContext.Client.Delete(r.kubegresContext.Ctx, current)
	}
	minAvailable := int32(1)
	if kubegres.Spec.PodDisruptionBudget.MinAvailable != nil {
		minAvailable = *kubegres.Spec.PodDisruptionBudget.MinAvailable
	}
	desired := &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: kubegres.Namespace, OwnerReferences: []metav1.OwnerReference{{APIVersion: postgresV1.GroupVersion.String(), Kind: "Kubegres", Name: kubegres.Name, UID: kubegres.UID}}},
		Spec:       policy.PodDisruptionBudgetSpec{MinAvailable: func() *intstr.IntOrString { v := intstr.FromInt(int(minAvailable)); return &v }(), Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": kubegres.Name}}},
	}
	if apierrors.IsNotFound(err) {
		return r.kubegresContext.Client.Create(r.kubegresContext.Ctx, desired)
	}
	current.Spec = desired.Spec
	return r.kubegresContext.Client.Update(r.kubegresContext.Ctx, current)
}

func (r *ServicesCountSpecEnforcer) isPrimaryServiceDeployed() bool {
	return r.resourcesStates.Services.Primary.IsDeployed
}

func (r *ServicesCountSpecEnforcer) isReplicaServiceDeployed() bool {
	return r.resourcesStates.Services.Replica.IsDeployed
}

func (r *ServicesCountSpecEnforcer) isPrimaryDbReady() bool {
	return r.resourcesStates.StatefulSets.Primary.IsReady
}

func (r *ServicesCountSpecEnforcer) isThereReadyReplica() bool {
	return r.resourcesStates.StatefulSets.Replicas.NbreReady > 0
}

func (r *ServicesCountSpecEnforcer) deployPrimaryService() error {
	return r.deployService(true)
}

func (r *ServicesCountSpecEnforcer) deployReplicaService() error {
	return r.deployService(false)
}

func (r *ServicesCountSpecEnforcer) deployService(isPrimary bool) error {

	primaryOrReplicaTxt := r.createLogLabel(isPrimary)

	service, err := r.createServiceResource(isPrimary)
	if err != nil {
		r.kubegresContext.Log.ErrorEvent("ServiceTemplateErr", err, "Unable to create "+primaryOrReplicaTxt+" Service object from template.")
		return err
	}

	if err := r.kubegresContext.Client.Create(r.kubegresContext.Ctx, &service); err != nil {
		r.kubegresContext.Log.ErrorEvent("ServiceDeploymentErr", err, "Unable to deploy "+primaryOrReplicaTxt+" Service.", "Service name", service.Name)
		return err
	}

	r.kubegresContext.Log.InfoEvent("ServiceDeployment", "Deployed "+primaryOrReplicaTxt+" Service.", "Service name", service.Name)
	return nil
}

func (r *ServicesCountSpecEnforcer) createLogLabel(isPrimary bool) string {
	if isPrimary {
		return "Primary"
	} else {
		return "Replica"
	}
}

func (r *ServicesCountSpecEnforcer) createServiceResource(isPrimary bool) (core.Service, error) {
	if isPrimary {
		return r.resourcesCreator.CreatePrimaryService()
	} else {
		return r.resourcesCreator.CreateReplicaService()
	}
}
