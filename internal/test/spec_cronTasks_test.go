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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
	resourceConfigs2 "reactive-tech.io/kubegres/internal/test/resourceConfigs"
	util2 "reactive-tech.io/kubegres/internal/test/util"
)

// cronTaskCronJobName mirrors the deterministic naming scheme documented in
// docs/cron-tasks.md and implemented by CronTasksCountSpecEnforcer: "<kubegres>-task-<task>".
func cronTaskCronJobName(kubegresName, taskName string) string {
	return kubegresName + "-task-" + taskName
}

var _ = Describe("Setting Kubegres spec 'cronTasks'", Label("storage-config", "shard-6"), func() {

	var test = SpecCronTasksTest{}

	BeforeEach(func() {
		//Skip("Temporarily skipping test")

		namespace := resourceConfigs2.DefaultNamespace
		test.resourceRetriever = util2.CreateTestResourceRetriever(k8sClientTest, namespace)
		test.resourceCreator = util2.CreateTestResourceCreator(k8sClientTest, test.resourceRetriever, namespace)
	})

	AfterEach(func() {
		if !test.keepCreatedResourcesForNextTest {
			test.resourceCreator.DeleteAllTestResources()
		} else {
			test.keepCreatedResourcesForNextTest = false
		}
	})

	Context("GIVEN new Kubegres is created with one spec 'cronTasks' entry", func() {

		It("THEN 1 primary is deployed AND a CronJob is created with the expected schedule, image and command", func() {

			log.Print("START OF: Test 'GIVEN new Kubegres is created with one spec 'cronTasks' entry'")

			vacuumTask := test.givenCronTask("vacuum", "*/5 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"vacuumdb --all"})

			test.givenNewKubegresSpecIsSetTo([]postgresv1.KubegresCronTask{vacuumTask}, 1)

			test.whenKubegresIsCreated()

			test.thenPodsStatesShouldBe(1, 0)

			test.thenCronJobExistsWithSpec("vacuum", "*/5 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"vacuumdb --all"})

			test.keepCreatedResourcesForNextTest = true

			log.Print("END OF: Test 'GIVEN new Kubegres is created with one spec 'cronTasks' entry'")
		})
	})

	Context("GIVEN existing Kubegres has one spec 'cronTasks' entry AND later a second entry is added and the first is removed", func() {

		It("THEN both CronJobs exist once the second entry is added AND only the second CronJob remains once the first entry is removed", func() {

			log.Print("START OF: Test 'GIVEN existing Kubegres has one spec 'cronTasks' entry AND later a second entry is added and the first is removed'")

			vacuumTask := test.givenCronTask("vacuum", "*/5 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"vacuumdb --all"})
			reindexTask := test.givenCronTask("reindex", "*/10 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"reindexdb --all"})

			test.givenExistingKubegresSpecIsSetTo([]postgresv1.KubegresCronTask{vacuumTask, reindexTask})

			test.whenKubernetesIsUpdated()

			test.thenCronJobExistsWithSpec("vacuum", "*/5 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"vacuumdb --all"})
			test.thenCronJobExistsWithSpec("reindex", "*/10 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"reindexdb --all"})

			test.givenExistingKubegresSpecIsSetTo([]postgresv1.KubegresCronTask{reindexTask})

			test.whenKubernetesIsUpdated()

			test.thenCronJobDoesNOTExist("vacuum")
			test.thenCronJobExistsWithSpec("reindex", "*/10 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"reindexdb --all"})

			log.Print("END OF: Test 'GIVEN existing Kubegres has one spec 'cronTasks' entry AND later a second entry is added and the first is removed'")
		})
	})

	Context("GIVEN Kubegres has spec 'cronTasks' configured AND later Kubegres is deleted", func() {

		It("THEN the owned CronJob is also deleted", func() {

			log.Print("START OF: Test 'GIVEN Kubegres has spec 'cronTasks' configured AND later Kubegres is deleted'")

			analyseTask := test.givenCronTask("analyse", "*/15 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"analyse.sh"})

			test.givenNewKubegresSpecIsSetTo([]postgresv1.KubegresCronTask{analyseTask}, 1)

			test.whenKubegresIsCreated()

			test.thenPodsStatesShouldBe(1, 0)

			test.thenCronJobExistsWithSpec("analyse", "*/15 * * * *", "postgres:17", []string{"sh", "-c"}, []string{"analyse.sh"})

			test.whenKubegresIsDeleted()

			test.thenCronJobDoesNOTExist("analyse")

			log.Print("END OF: Test 'GIVEN Kubegres has spec 'cronTasks' configured AND later Kubegres is deleted'")
		})
	})

})

type SpecCronTasksTest struct {
	keepCreatedResourcesForNextTest bool
	kubegresResource                *postgresv1.Kubegres
	resourceCreator                 util2.TestResourceCreator
	resourceRetriever               util2.TestResourceRetriever
}

func (r *SpecCronTasksTest) givenCronTask(name, schedule, image string, command, args []string) postgresv1.KubegresCronTask {
	return postgresv1.KubegresCronTask{
		Name:     name,
		Schedule: schedule,
		Image:    image,
		Command:  command,
		Args:     args,
	}
}

func (r *SpecCronTasksTest) givenNewKubegresSpecIsSetTo(cronTasks []postgresv1.KubegresCronTask, specNbreReplicas int32) {
	r.kubegresResource = resourceConfigs2.LoadKubegresYaml()
	r.kubegresResource.Spec.CronTasks = cronTasks
	r.kubegresResource.Spec.Replicas = &specNbreReplicas
}

func (r *SpecCronTasksTest) givenExistingKubegresSpecIsSetTo(cronTasks []postgresv1.KubegresCronTask) {
	var err error
	r.kubegresResource, err = r.resourceRetriever.GetKubegres()

	if err != nil {
		log.Println("Error while getting Kubegres resource : ", err)
		Expect(err).Should(Succeed())
		return
	}

	r.kubegresResource.Spec.CronTasks = cronTasks
}

func (r *SpecCronTasksTest) whenKubegresIsCreated() {
	r.resourceCreator.CreateKubegres(r.kubegresResource)
}

func (r *SpecCronTasksTest) whenKubernetesIsUpdated() {
	r.resourceCreator.UpdateResource(r.kubegresResource, "Kubegres")
}

func (r *SpecCronTasksTest) whenKubegresIsDeleted() {
	var err error
	r.kubegresResource, err = r.resourceRetriever.GetKubegres()
	Expect(err).Should(Succeed())
	r.resourceCreator.DeleteResource(r.kubegresResource, "Kubegres")
}

func (r *SpecCronTasksTest) thenPodsStatesShouldBe(nbrePrimary, nbreReplicas int) bool {
	return Eventually(func() bool {

		kubegresResources, err := r.resourceRetriever.GetKubegresResources()
		if err != nil && !apierrors.IsNotFound(err) {
			log.Println("ERROR while retrieving Kubegres kubegresResources")
			return false
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

func (r *SpecCronTasksTest) thenCronJobExistsWithSpec(taskName, expectedSchedule, expectedImage string, expectedCommand, expectedArgs []string) bool {

	cronJobName := cronTaskCronJobName(resourceConfigs2.KubegresResourceName, taskName)

	return Eventually(func() bool {

		cronJob, err := r.resourceRetriever.GetCronJobByName(cronJobName)
		if err != nil {
			log.Println("CronJob '" + cronJobName + "' not found yet. Waiting... Error: " + err.Error())
			return false
		}

		if cronJob.Spec.Schedule != expectedSchedule {
			log.Println("CronJob '" + cronJobName + "' doesn't have the expected schedule: '" + expectedSchedule + "'. Waiting...")
			return false
		}

		container := cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
		if container.Image != expectedImage {
			log.Println("CronJob '" + cronJobName + "' doesn't have the expected image: '" + expectedImage + "'. Waiting...")
			return false
		}

		if !stringSlicesEqual(container.Command, expectedCommand) {
			log.Println("CronJob '" + cronJobName + "' doesn't have the expected command. Waiting...")
			return false
		}

		if !stringSlicesEqual(container.Args, expectedArgs) {
			log.Println("CronJob '" + cronJobName + "' doesn't have the expected args. Waiting...")
			return false
		}

		return true

	}, time.Second*30, time.Second*5).Should(BeTrue())
}

func (r *SpecCronTasksTest) thenCronJobDoesNOTExist(taskName string) bool {

	cronJobName := cronTaskCronJobName(resourceConfigs2.KubegresResourceName, taskName)

	return Eventually(func() bool {

		_, err := r.resourceRetriever.GetCronJobByName(cronJobName)
		if apierrors.IsNotFound(err) {
			return true
		}
		if err != nil {
			log.Println("ERROR while retrieving CronJob '" + cronJobName + "': " + err.Error())
			return false
		}

		log.Println("CronJob '" + cronJobName + "' should be deleted. Waiting...")
		return false

	}, time.Second*30, time.Second*5).Should(BeTrue())
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
