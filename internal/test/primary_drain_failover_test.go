/*
Copyright 2026 Reactive Tech Limited.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package test

import (
	"context"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	postgresv1 "reactive-tech.io/kubegres/api/v1"
	resourceConfigs2 "reactive-tech.io/kubegres/internal/test/resourceConfigs"
	util2 "reactive-tech.io/kubegres/internal/test/util"
)

var _ = Describe("Primary voluntary disruption failover", Label("core-failover", "shard-5"), func() {
	var resourceRetriever util2.TestResourceRetriever
	var resourceCreator util2.TestResourceCreator

	BeforeEach(func() {
		namespace := resourceConfigs2.DefaultNamespace
		resourceRetriever = util2.CreateTestResourceRetriever(k8sClientTest, namespace)
		resourceCreator = util2.CreateTestResourceCreator(k8sClientTest, resourceRetriever, namespace)
	})

	AfterEach(func() {
		resourceCreator.DeleteAllTestResources()
	})

	It("promotes the healthy replica when the primary is evicted and rebuilds the old primary", func() {
		kubegres := resourceConfigs2.LoadKubegresYaml()
		replicaCount := int32(2)
		kubegres.Spec.Replicas = &replicaCount
		kubegres.Spec.Failover = postgresv1.KubegresFailover{OnPrimaryPodDrain: true}

		resourceCreator.CreateKubegres(kubegres)

		var initial util2.TestKubegresResources
		Eventually(func() bool {
			var err error
			initial, err = resourceRetriever.GetKubegresResources()
			return err == nil && initial.NbreDeployedPrimary == 1 &&
				initial.NbreDeployedReplicas == 1 && initial.AreAllReady
		}, 5*time.Minute, 5*time.Second).Should(BeTrue())

		primaryPodName, replicaPodName := podNamesByRole(initial)
		Expect(primaryPodName).NotTo(BeEmpty())
		Expect(replicaPodName).NotTo(BeEmpty())

		By("verifying a PodDisruptionBudget protects the cluster while failover.onPrimaryPodDrain is enabled")
		Eventually(func() bool {
			pdb, err := resourceRetriever.GetPodDisruptionBudget(kubegres.Name)
			if err != nil {
				return false
			}
			if pdb.Spec.Selector == nil || pdb.Spec.Selector.MatchLabels["app"] != kubegres.Name {
				return false
			}
			return pdb.Spec.MinAvailable != nil && pdb.Spec.MinAvailable.IntValue() == 1
		}, time.Minute, 5*time.Second).Should(BeTrue())

		eviction := &policyv1beta1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      primaryPodName,
				Namespace: resourceConfigs2.DefaultNamespace,
			},
		}
		log.Printf("Evicting primary Pod %q", primaryPodName)
		Expect(k8sClientsetTest.CoreV1().Pods(resourceConfigs2.DefaultNamespace).
			Evict(context.Background(), eviction)).To(Succeed())

		Eventually(func() bool {
			resources, err := resourceRetriever.GetKubegresResources()
			if err != nil || resources.NbreDeployedPrimary != 1 ||
				resources.NbreDeployedReplicas != 1 || !resources.AreAllReady {
				return false
			}

			newPrimaryPodName, newReplicaPodName := podNamesByRole(resources)
			return newPrimaryPodName == replicaPodName && newReplicaPodName != replicaPodName
		}, 10*time.Minute, 5*time.Second).Should(BeTrue())

		By("disabling failover.onPrimaryPodDrain and verifying the PodDisruptionBudget is removed")
		var current *postgresv1.Kubegres
		Eventually(func() error {
			var err error
			current, err = resourceRetriever.GetKubegres()
			return err
		}, time.Minute, 5*time.Second).Should(Succeed())

		current.Spec.Failover.OnPrimaryPodDrain = false
		resourceCreator.UpdateResource(current, "Kubegres")

		Eventually(func() bool {
			_, err := resourceRetriever.GetPodDisruptionBudget(kubegres.Name)
			return apierrors.IsNotFound(err)
		}, time.Minute, 5*time.Second).Should(BeTrue())
	})
})

func podNamesByRole(resources util2.TestKubegresResources) (primaryPodName, replicaPodName string) {
	for _, resource := range resources.Resources {
		if resource.IsPrimary {
			primaryPodName = resource.Pod.Name
		} else {
			replicaPodName = resource.Pod.Name
		}
	}
	return primaryPodName, replicaPodName
}
