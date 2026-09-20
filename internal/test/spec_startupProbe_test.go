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

package test

import (
	"log"
	resourceConfigs2 "reactive-tech.io/kubegres/internal/test/resourceConfigs"
	util2 "reactive-tech.io/kubegres/internal/test/util"
	"reactive-tech.io/kubegres/internal/test/util/testcases"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v12 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
)

var _ = Describe("Setting Kubegres spec 'startupProbe'", Label("database-specs", "shard-10"), func() {

	var test = SpecStartupProbeTest{}

	BeforeEach(func() {
		//Skip("Temporarily skipping test")

		namespace := resourceConfigs2.DefaultNamespace
		test.resourceRetriever = util2.CreateTestResourceRetriever(k8sClientTest, namespace)
		test.resourceCreator = util2.CreateTestResourceCreator(k8sClientTest, test.resourceRetriever, namespace)
		test.dbQueryTestCases = testcases.InitDbQueryTestCases(test.resourceCreator, resourceConfigs2.KubegresResourceName)
	})

	AfterEach(func() {
		if !test.keepCreatedResourcesForNextTest {
			test.resourceCreator.DeleteAllTestResources()
		} else {
			test.keepCreatedResourcesForNextTest = false
		}
	})

	Context("GIVEN new Kubegres is created with spec 'startupProbe' set to a value and spec 'replica' set to 3 and later 'startupProbe' is updated to a new value", func() {

		It("GIVEN new Kubegres is created with spec 'startupProbe' set to a value and spec 'replica' set to 3 THEN 1 primary and 2 replica should be created with spec 'startupProbe' set the value", func() {

			log.Print("START OF: Test 'GIVEN new Kubegres is created with spec 'startupProbe' set to a value and spec 'replica' set to 3")

			startupProbe := test.givenStartupProbe1()

			test.givenNewKubegresSpecIsSetTo(startupProbe, 3)

			test.whenKubegresIsCreated()

			test.thenStatefulSetStatesShouldBe(startupProbe, 1, 2)

			test.thenDeployedKubegresSpecShouldBeSetTo(startupProbe)

			test.dbQueryTestCases.ThenWeCanSqlQueryPrimaryDb()
			test.dbQueryTestCases.ThenWeCanSqlQueryReplicaDb()

			test.keepCreatedResourcesForNextTest = true

			log.Print("END OF: Test 'GIVEN new Kubegres is created with spec 'startupProbe' set to a value and spec 'replica' set to 3'")
		})

		It("GIVEN existing Kubegres is updated with spec 'startupProbe' set to a new value THEN 1 primary and 2 replica should be re-deployed with spec 'startupProbe' set the new value", func() {

			log.Print("START OF: Test 'GIVEN existing Kubegres is updated with spec 'startupProbe' set to a new value")

			newStartupProbe := test.givenStartupProbe2()

			test.givenExistingKubegresSpecIsSetTo(newStartupProbe)

			test.whenKubernetesIsUpdated()

			test.thenStatefulSetStatesShouldBe(newStartupProbe, 1, 2)

			test.thenDeployedKubegresSpecShouldBeSetTo(newStartupProbe)

			test.dbQueryTestCases.ThenWeCanSqlQueryPrimaryDb()
			test.dbQueryTestCases.ThenWeCanSqlQueryReplicaDb()

			log.Print("END OF: Test 'GIVEN existing Kubegres is updated with spec 'startupProbe' set to a new value")
		})
	})

})

type SpecStartupProbeTest struct {
	keepCreatedResourcesForNextTest bool
	kubegresResource                *postgresv1.Kubegres
	dbQueryTestCases                testcases.DbQueryTestCases
	resourceCreator                 util2.TestResourceCreator
	resourceRetriever               util2.TestResourceRetriever
}

func (r *SpecStartupProbeTest) whenKubernetesIsUpdated() {
	r.resourceCreator.UpdateResource(r.kubegresResource, "Kubegres")
}

func (r *SpecStartupProbeTest) givenStartupProbe1() *v12.Probe {
	command := []string{"sh", "-c", "exec pg_isready -U postgres -h $POD_IP"}
	execAction := &v12.ExecAction{Command: command}
	handler := v12.ProbeHandler{Exec: execAction}
	return &v12.Probe{
		ProbeHandler:        handler,
		InitialDelaySeconds: int32(10),
		TimeoutSeconds:      int32(5),
		PeriodSeconds:       int32(10),
		SuccessThreshold:    int32(1),
		FailureThreshold:    int32(30),
	}
}

func (r *SpecStartupProbeTest) givenStartupProbe2() *v12.Probe {
	command := []string{"sh", "-c", "exec pg_isready -U postgres -h $POD_IP"}
	execAction := &v12.ExecAction{Command: command}
	handler := v12.ProbeHandler{Exec: execAction}
	return &v12.Probe{
		ProbeHandler:        handler,
		InitialDelaySeconds: int32(15),
		TimeoutSeconds:      int32(10),
		PeriodSeconds:       int32(20),
		SuccessThreshold:    int32(1),
		FailureThreshold:    int32(40),
	}
}

func (r *SpecStartupProbeTest) givenNewKubegresSpecIsSetTo(startupProbe *v12.Probe, specNbreReplicas int32) {
	r.kubegresResource = resourceConfigs2.LoadKubegresYaml()
	r.kubegresResource.Spec.Probe.StartupProbe = startupProbe
	r.kubegresResource.Spec.Replicas = &specNbreReplicas
}

func (r *SpecStartupProbeTest) givenExistingKubegresSpecIsSetTo(startupProbe *v12.Probe) {
	var err error
	r.kubegresResource, err = r.resourceRetriever.GetKubegres()

	if err != nil {
		log.Println("Error while getting Kubegres resource : ", err)
		Expect(err).Should(Succeed())
		return
	}

	r.kubegresResource.Spec.Probe.StartupProbe = startupProbe
}

func (r *SpecStartupProbeTest) whenKubegresIsCreated() {
	r.resourceCreator.CreateKubegres(r.kubegresResource)
}

func (r *SpecStartupProbeTest) thenStatefulSetStatesShouldBe(expectedProbe *v12.Probe, nbrePrimary, nbreReplicas int) bool {
	return Eventually(func() bool {

		kubegresResources, err := r.resourceRetriever.GetKubegresResources()
		if err != nil && !apierrors.IsNotFound(err) {
			log.Println("ERROR while retrieving Kubegres kubegresResources")
			return false
		}

		for _, resource := range kubegresResources.Resources {
			currentProbe := resource.StatefulSet.Spec.Template.Spec.Containers[0].StartupProbe

			if !reflect.DeepEqual(currentProbe, expectedProbe) {
				log.Println("StatefulSet '" + resource.StatefulSet.Name + "' doesn't have the expected spec 'startupProbe': " + expectedProbe.String() + " " +
					"Current value: '" + currentProbe.String() + "'. Waiting...")
				return false
			}
		}

		if kubegresResources.AreAllReady &&
			kubegresResources.NbreDeployedPrimary == nbrePrimary &&
			kubegresResources.NbreDeployedReplicas == nbreReplicas {

			time.Sleep(resourceConfigs2.TestRetryInterval)
			log.Println("Deployed and Ready StatefulSets check successful")
			return true
		}

		return false

	}, resourceConfigs2.TestTimeout, resourceConfigs2.TestRetryInterval).Should(BeTrue())
}

func (r *SpecStartupProbeTest) thenDeployedKubegresSpecShouldBeSetTo(expectedProbe *v12.Probe) {
	var err error
	r.kubegresResource, err = r.resourceRetriever.GetKubegres()

	if err != nil {
		log.Println("Error while getting Kubegres resource : ", err)
		Expect(err).Should(Succeed())
		return
	}

	currentProbe := r.kubegresResource.Spec.Probe.StartupProbe
	Expect(currentProbe).Should(Equal(expectedProbe))
}
