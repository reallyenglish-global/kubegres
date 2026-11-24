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

package kindcluster

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const clusterName = "kubegres"

// PrepareEnvironment creates or reuses the named disposable cluster and binds
// envtest before Start can install CRDs. The kubeconfig for diagnostics
// must be in a private suite directory, never the caller's default kubeconfig.
func PrepareEnvironment(kubeconfig, crdDir string) (*envtest.Environment, error) {
	clusters, err := exec.Command("kind", "get", "clusters").Output()
	if err != nil {
		return nil, fmt.Errorf("list Kind clusters: %w", err)
	}
	if !slices.Contains(strings.Fields(string(clusters)), clusterName) {
		cmd := exec.Command("kind", "create", "cluster", "--name", clusterName, "--config", "util/kindcluster/kind-cluster-config.yaml", "--kubeconfig", kubeconfig)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("create Kind cluster: %w", err)
		}
	}
	data, err := exec.Command("kind", "get", "kubeconfig", "--name", clusterName).Output()
	if err != nil {
		return nil, fmt.Errorf("get kubeconfig for Kind cluster %q: %w", clusterName, err)
	}
	cfg, err := namedConfig(data)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(kubeconfig, data, 0600); err != nil {
		return nil, err
	}
	cfg.Timeout = 30 * time.Second
	useExistingCluster := true
	return &envtest.Environment{
		Config:                cfg,
		UseExistingCluster:    &useExistingCluster,
		CRDDirectoryPaths:     []string{crdDir},
		ErrorIfCRDPathMissing: true,
	}, nil
}

func DeleteCluster(kubeconfig string) error {
	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName, "--kubeconfig", kubeconfig)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
