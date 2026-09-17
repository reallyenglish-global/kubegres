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
	"k8s.io/client-go/tools/record"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reactive-tech.io/kubegres/internal/controller"
	util2 "reactive-tech.io/kubegres/internal/test/util"
	"reactive-tech.io/kubegres/internal/test/util/kindcluster"
	"runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	postgresv1 "reactive-tech.io/kubegres/api/v1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var kindCluster kindcluster.KindTestClusterUtil

// var cfgTest *rest.Config
var k8sClientTest client.Client
var testEnv *envtest.Environment
var eventRecorderTest util2.MockEventRecorderTestUtil

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {

	kindCluster.StartCluster()

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	log.Print("START OF: BeforeSuite")

	By("bootstrapping test environment")
	useExistingCluster := true
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &useExistingCluster,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.35.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	// Keep individual Kubernetes API requests bounded so an unavailable API server
	// cannot block an Eventually assertion past its test timeout.
	cfg.Timeout = 30 * time.Second

	err = postgresv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClientTest, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClientTest).ToNot(BeNil())

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
	output, err := exec.CommandContext(ctx, "python3", "../../hack/collect_test_diagnostics.py", dest).CombinedOutput()
	fmt.Fprintf(GinkgoWriter, "Failure diagnostics: %s (%v)\n%s", dest, err, output)
})

var _ = AfterSuite(func() {

	log.Print("START OF: Suite AfterSuite")

	By("tearing down the test environment")

	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())

	time.Sleep(5 * time.Second)

	if os.Getenv("KUBEGRES_KEEP_TEST_CLUSTER") != "true" {
		kindCluster.DeleteCluster()
	}

	log.Print("END OF: Suite AfterSuite")
})
