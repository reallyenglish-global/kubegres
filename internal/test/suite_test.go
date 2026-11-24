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
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	postgresv1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller"
	util2 "reactive-tech.io/kubegres/internal/test/util"
	"reactive-tech.io/kubegres/internal/test/util/kindcluster"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var suiteKubeconfig string
var k8sClientTest client.Client
var k8sClientsetTest kubernetes.Interface
var testEnv *envtest.Environment
var eventRecorderTest util2.MockEventRecorderTestUtil

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	log.Print("START OF: BeforeSuite")

	By("bootstrapping test environment")
	suiteKubeconfig = filepath.Join(GinkgoT().TempDir(), "kubeconfig")
	var err error
	testEnv, err = kindcluster.PrepareEnvironment(suiteKubeconfig, filepath.Join("..", "..", "config", "crd", "bases"))
	Expect(err).NotTo(HaveOccurred())
	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = postgresv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClientTest, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClientTest).ToNot(BeNil())

	k8sClientsetTest, err = kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClientsetTest).ToNot(BeNil())

	// Kind's local-path StorageClass leaves AllowVolumeExpansion unset. The
	// database-size feature is exercised against the default test class, so
	// explicitly enable the capability that a production CSI class must provide.
	standardStorageClass, err := k8sClientsetTest.StorageV1().StorageClasses().Get(context.Background(), "standard", metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	allowVolumeExpansion := true
	standardStorageClass.AllowVolumeExpansion = &allowVolumeExpansion
	_, err = k8sClientsetTest.StorageV1().StorageClasses().Update(context.Background(), standardStorageClass, metav1.UpdateOptions{})
	Expect(err).ToNot(HaveOccurred())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	mockLogger := util2.CreateMockLogger()
	eventRecorderTest = util2.MockEventRecorderTestUtil{}

	err = (&controller.KubegresReconciler{
		Client:   k8sManager.GetClient(),
		Logger:   mockLogger,
		Scheme:   k8sManager.GetScheme(),
		Recorder: record.EventRecorder(&eventRecorderTest),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		if err != nil {
			log.Fatal("ERROR while starting Kubernetes: ", err)
		}
		Expect(err).ToNot(HaveOccurred())
	}()

	log.Println("Waiting for Kubernetes to start")
	log.Println("Kubernetes has started")

	// Keep test reads on the direct client created above. The manager client is
	// cache-backed and can block indefinitely when its informer cache stalls.
	Expect(k8sClientTest).ToNot(BeNil())

	log.Print("END OF: BeforeSuite")

})

// Run before AfterEach resource deletion, so the failing state is retained.
var _ = JustAfterEach(func() {
	if !CurrentSpecReport().Failed() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	dest := filepath.Join("..", "..", "artifacts", "failures", fmt.Sprint(time.Now().UnixNano()))
	output, err := exec.CommandContext(ctx, "python3", "../../hack/collect_test_diagnostics.py", dest, suiteKubeconfig).CombinedOutput()
	fmt.Fprintf(GinkgoWriter, "Failure diagnostics: %s (%v)\n%s", dest, err, output)
})

var _ = AfterSuite(func() {

	log.Print("START OF: Suite AfterSuite")

	By("tearing down the test environment")

	if testEnv == nil { // Binding failed; do not run any cluster cleanup.
		return
	}
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())

	time.Sleep(5 * time.Second)

	if os.Getenv("KUBEGRES_KEEP_TEST_CLUSTER") != "true" {
		Expect(kindcluster.DeleteCluster(suiteKubeconfig)).To(Succeed())
	}

	log.Print("END OF: Suite AfterSuite")
})
