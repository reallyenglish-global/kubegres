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

var _ = Describe("Setting Kubegres spec 'lifecycle.preStop'", Label("database-specs", "shard-7"), func() {

	var test = SpecPreStopTest{}

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

	Context("GIVEN new Kubegres is created with spec 'lifecycle.preStop' set to a value and spec 'replica' set to 3 and later 'lifecycle.preStop' is updated to a new value", func() {

		It("GIVEN new Kubegres is created with spec 'lifecycle.preStop' set to a value and spec 'replica' set to 3 THEN 1 primary and 2 replica should be created with spec 'lifecycle.preStop' set the value", func() {

			log.Print("START OF: Test 'GIVEN new Kubegres is created with spec 'lifecycle.preStop' set to a value and spec 'replica' set to 3")

			preStop := test.givenPreStop1()

			test.givenNewKubegresSpecIsSetTo(preStop, 3)

			test.whenKubegresIsCreated()

			test.thenStatefulSetStatesShouldBe(preStop, 1, 2)

			test.thenDeployedKubegresSpecShouldBeSetTo(preStop)

			test.dbQueryTestCases.ThenWeCanSqlQueryPrimaryDb()
			test.dbQueryTestCases.ThenWeCanSqlQueryReplicaDb()

			test.keepCreatedResourcesForNextTest = true

			log.Print("END OF: Test 'GIVEN new Kubegres is created with spec 'lifecycle.preStop' set to a value and spec 'replica' set to 3'")
		})

		It("GIVEN existing Kubegres is updated with spec 'lifecycle.preStop' set to a new value THEN 1 primary and 2 replica should be re-deployed with spec 'lifecycle.preStop' set the new value", func() {

			log.Print("START OF: Test 'GIVEN existing Kubegres is updated with spec 'lifecycle.preStop' set to a new value")

			newPreStop := test.givenPreStop2()

			test.givenExistingKubegresSpecIsSetTo(newPreStop)

			test.whenKubernetesIsUpdated()

			test.thenStatefulSetStatesShouldBe(newPreStop, 1, 2)

			test.thenDeployedKubegresSpecShouldBeSetTo(newPreStop)

			test.dbQueryTestCases.ThenWeCanSqlQueryPrimaryDb()
			test.dbQueryTestCases.ThenWeCanSqlQueryReplicaDb()

			log.Print("END OF: Test 'GIVEN existing Kubegres is updated with spec 'lifecycle.preStop' set to a new value")
		})
	})

})

type SpecPreStopTest struct {
	keepCreatedResourcesForNextTest bool
	kubegresResource                *postgresv1.Kubegres
	dbQueryTestCases                testcases.DbQueryTestCases
	resourceCreator                 util2.TestResourceCreator
	resourceRetriever               util2.TestResourceRetriever
}

func (r *SpecPreStopTest) whenKubernetesIsUpdated() {
	r.resourceCreator.UpdateResource(r.kubegresResource, "Kubegres")
}

func (r *SpecPreStopTest) givenPreStop1() *v12.LifecycleHandler {
	command := []string{"sh", "-c", "echo custom-pre-stop-1 && pg_ctl -D $PGDATA stop -m fast"}
	execAction := &v12.ExecAction{Command: command}
	return &v12.LifecycleHandler{Exec: execAction}
}

func (r *SpecPreStopTest) givenPreStop2() *v12.LifecycleHandler {
	command := []string{"sh", "-c", "echo custom-pre-stop-2 && pg_ctl -D $PGDATA stop -m smart"}
	execAction := &v12.ExecAction{Command: command}
	return &v12.LifecycleHandler{Exec: execAction}
}

func (r *SpecPreStopTest) givenNewKubegresSpecIsSetTo(preStop *v12.LifecycleHandler, specNbreReplicas int32) {
	r.kubegresResource = resourceConfigs2.LoadKubegresYaml()
	r.kubegresResource.Spec.Lifecycle.PreStop = preStop
	r.kubegresResource.Spec.Replicas = &specNbreReplicas
}

func (r *SpecPreStopTest) givenExistingKubegresSpecIsSetTo(preStop *v12.LifecycleHandler) {
	var err error
	r.kubegresResource, err = r.resourceRetriever.GetKubegres()

	if err != nil {
		log.Println("Error while getting Kubegres resource : ", err)
		Expect(err).Should(Succeed())
		return
	}

	r.kubegresResource.Spec.Lifecycle.PreStop = preStop
}

func (r *SpecPreStopTest) whenKubegresIsCreated() {
	r.resourceCreator.CreateKubegres(r.kubegresResource)
}

func (r *SpecPreStopTest) thenStatefulSetStatesShouldBe(expectedPreStop *v12.LifecycleHandler, nbrePrimary, nbreReplicas int) bool {
	return Eventually(func() bool {

		kubegresResources, err := r.resourceRetriever.GetKubegresResources()
		if err != nil && !apierrors.IsNotFound(err) {
			log.Println("ERROR while retrieving Kubegres kubegresResources")
			return false
		}

		for _, resource := range kubegresResources.Resources {
			currentPreStop := resource.StatefulSet.Spec.Template.Spec.Containers[0].Lifecycle.PreStop

			if !reflect.DeepEqual(currentPreStop, expectedPreStop) {
				log.Println("StatefulSet '" + resource.StatefulSet.Name + "' doesn't have the expected spec 'lifecycle.preStop': " + expectedPreStop.String() + " " +
					"Current value: '" + currentPreStop.String() + "'. Waiting...")
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

func (r *SpecPreStopTest) thenDeployedKubegresSpecShouldBeSetTo(expectedPreStop *v12.LifecycleHandler) {
	var err error
	r.kubegresResource, err = r.resourceRetriever.GetKubegres()

	if err != nil {
		log.Println("Error while getting Kubegres resource : ", err)
		Expect(err).Should(Succeed())
		return
	}

	currentPreStop := r.kubegresResource.Spec.Lifecycle.PreStop
	Expect(currentPreStop).Should(Equal(expectedPreStop))
}
