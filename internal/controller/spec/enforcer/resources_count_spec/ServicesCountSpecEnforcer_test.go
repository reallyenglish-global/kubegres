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
	"context"
	"testing"

	policy "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newPdbTestKubegres(name string) *postgresV1.Kubegres {
	return &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name + "-uid")},
	}
}

func newPdbTestEnforcer(t *testing.T, kubegres *postgresV1.Kubegres) (ServicesCountSpecEnforcer, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := policy.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	enforcer := ServicesCountSpecEnforcer{
		kubegresContext: ctx.KubegresContext{Ctx: context.Background(), Client: kubeClient, Kubegres: kubegres},
	}
	return enforcer, kubeClient
}

func getPdb(t *testing.T, kubeClient client.Client, name string) (*policy.PodDisruptionBudget, error) {
	t.Helper()
	pdb := &policy.PodDisruptionBudget{}
	err := kubeClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: "default"}, pdb)
	return pdb, err
}

// Proves that when the PDB is disabled and onPrimaryPodDrain is false, no PodDisruptionBudget
// is created.
func TestEnsurePodDisruptionBudget_DisabledAndNoDrain_NoPdbCreated(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	if _, err := getPdb(t, kubeClient, "database"); !apierrors.IsNotFound(err) {
		t.Fatalf("expected no PodDisruptionBudget to be created, got err=%v", err)
	}
}

// Proves that when the PDB is disabled and onPrimaryPodDrain is false, an existing
// PodDisruptionBudget is removed by the enforcer.
func TestEnsurePodDisruptionBudget_DisabledAndNoDrain_ExistingPdbIsRemoved(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	existing := &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "default"},
		Spec: policy.PodDisruptionBudgetSpec{
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "database"}},
		},
	}
	if err := kubeClient.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	if _, err := getPdb(t, kubeClient, "database"); !apierrors.IsNotFound(err) {
		t.Fatalf("expected existing PodDisruptionBudget to be deleted, got err=%v", err)
	}
}

// Proves that spec.podDisruptionBudget.enabled=true creates a PDB with the expected
// selector, owner labels, and a default minAvailable of 1.
func TestEnsurePodDisruptionBudget_Enabled_CreatesPdbWithDefaults(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	pdb, err := getPdb(t, kubeClient, "database")
	if err != nil {
		t.Fatal(err)
	}
	if pdb.Spec.Selector == nil || pdb.Spec.Selector.MatchLabels["app"] != "database" {
		t.Fatalf("expected selector to match app=database, got %#v", pdb.Spec.Selector)
	}
	if pdb.Spec.MinAvailable == nil || pdb.Spec.MinAvailable.IntValue() != 1 {
		t.Fatalf("expected default minAvailable of 1, got %#v", pdb.Spec.MinAvailable)
	}
	if len(pdb.OwnerReferences) != 1 || pdb.OwnerReferences[0].Name != "database" || pdb.OwnerReferences[0].UID != kubegres.UID {
		t.Fatalf("expected owner reference to the Kubegres resource, got %#v", pdb.OwnerReferences)
	}
}

// Proves that onPrimaryPodDrain=true alone (without podDisruptionBudget.enabled) is
// sufficient to create the PDB.
func TestEnsurePodDisruptionBudget_OnPrimaryPodDrainAlone_CreatesPdb(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.Failover.OnPrimaryPodDrain = true
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	if _, err := getPdb(t, kubeClient, "database"); err != nil {
		t.Fatalf("expected PodDisruptionBudget to be created, got err=%v", err)
	}
}

// Proves that an explicit spec.podDisruptionBudget.minAvailable value is propagated to
// the created PDB instead of the default.
func TestEnsurePodDisruptionBudget_ExplicitMinAvailable_IsPropagated(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	explicitMin := int32(3)
	kubegres.Spec.PodDisruptionBudget.MinAvailable = &explicitMin
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	pdb, err := getPdb(t, kubeClient, "database")
	if err != nil {
		t.Fatal(err)
	}
	if pdb.Spec.MinAvailable == nil || pdb.Spec.MinAvailable.IntValue() != 3 {
		t.Fatalf("expected minAvailable of 3, got %#v", pdb.Spec.MinAvailable)
	}
}

// Proves that the created PDB's owner reference is a controller reference
// (Controller=true), which controller-runtime's Owns() requires to enqueue
// reconciles for changes to owned PodDisruptionBudgets.
func TestEnsurePodDisruptionBudget_Enabled_OwnerReferenceIsController(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	pdb, err := getPdb(t, kubeClient, "database")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdb.OwnerReferences) != 1 {
		t.Fatalf("expected exactly one owner reference, got %#v", pdb.OwnerReferences)
	}
	owner := pdb.OwnerReferences[0]
	if owner.Controller == nil || !*owner.Controller {
		t.Fatalf("expected owner reference Controller=true, got %#v", owner)
	}
	if owner.BlockOwnerDeletion == nil || !*owner.BlockOwnerDeletion {
		t.Fatalf("expected owner reference BlockOwnerDeletion=true, got %#v", owner)
	}
}

// Proves that spec.podDisruptionBudget.unhealthyPodEvictionPolicy is propagated
// to the PDB when set, and left unset (nil, so the API server default applies)
// otherwise.
func TestEnsurePodDisruptionBudget_UnhealthyPodEvictionPolicy_IsPropagated(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	policyValue := "AlwaysAllow"
	kubegres.Spec.PodDisruptionBudget.UnhealthyPodEvictionPolicy = &policyValue
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	pdb, err := getPdb(t, kubeClient, "database")
	if err != nil {
		t.Fatal(err)
	}
	if pdb.Spec.UnhealthyPodEvictionPolicy == nil || *pdb.Spec.UnhealthyPodEvictionPolicy != policy.AlwaysAllow {
		t.Fatalf("expected unhealthyPodEvictionPolicy AlwaysAllow, got %#v", pdb.Spec.UnhealthyPodEvictionPolicy)
	}
}

func TestEnsurePodDisruptionBudget_UnhealthyPodEvictionPolicyUnset_LeavesDefaultBehaviourUnchanged(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	pdb, err := getPdb(t, kubeClient, "database")
	if err != nil {
		t.Fatal(err)
	}
	if pdb.Spec.UnhealthyPodEvictionPolicy != nil {
		t.Fatalf("expected unhealthyPodEvictionPolicy to remain unset, got %#v", pdb.Spec.UnhealthyPodEvictionPolicy)
	}
}

// Proves that an existing PDB with a spec that has drifted from the desired state
// (wrong minAvailable and selector) is updated to match the desired spec, in place.
func TestEnsurePodDisruptionBudget_ExistingPdbWithDriftedSpec_IsUpdated(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	explicitMin := int32(2)
	kubegres.Spec.PodDisruptionBudget.MinAvailable = &explicitMin
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	drifted := &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "default"},
		Spec: policy.PodDisruptionBudgetSpec{
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "some-other-app"}},
		},
	}
	if err := kubeClient.Create(context.Background(), drifted); err != nil {
		t.Fatal(err)
	}

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	pdb, err := getPdb(t, kubeClient, "database")
	if err != nil {
		t.Fatal(err)
	}
	if pdb.Spec.MinAvailable == nil || pdb.Spec.MinAvailable.IntValue() != 2 {
		t.Fatalf("expected drifted minAvailable to be corrected to 2, got %#v", pdb.Spec.MinAvailable)
	}
	if pdb.Spec.Selector == nil || pdb.Spec.Selector.MatchLabels["app"] != "database" {
		t.Fatalf("expected drifted selector to be corrected to app=database, got %#v", pdb.Spec.Selector)
	}
}

// Proves that reconciling an unchanged PDB spec performs zero Update calls,
// so a reconcile loop does not repeatedly write an unchanged PDB.
func TestEnsurePodDisruptionBudget_UnchangedSpec_PerformsNoUpdate(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	explicitMin := int32(2)
	kubegres.Spec.PodDisruptionBudget.MinAvailable = &explicitMin

	scheme := runtime.NewScheme()
	if err := policy.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	updateCount := 0
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			updateCount++
			return c.Update(ctx, obj, opts...)
		},
	}).Build()
	enforcer := ServicesCountSpecEnforcer{
		kubegresContext: ctx.KubegresContext{Ctx: context.Background(), Client: kubeClient, Kubegres: kubegres},
	}

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}
	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	if updateCount != 0 {
		t.Fatalf("expected zero Update calls for an unchanged PDB spec, got %d", updateCount)
	}
}

// Proves that an existing PDB whose owner reference is not a controller
// reference (for example, created before this field was set) is repaired to
// Controller=true so controller-runtime's Owns() enqueues reconciles for it.
func TestEnsurePodDisruptionBudget_ExistingPdbWithNonControllerOwnerRef_IsRepaired(t *testing.T) {
	kubegres := newPdbTestKubegres("database")
	kubegres.Spec.PodDisruptionBudget.Enabled = true
	enforcer, kubeClient := newPdbTestEnforcer(t, kubegres)

	existing := &policy.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name: "database", Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: postgresV1.GroupVersion.String(), Kind: "Kubegres", Name: "database", UID: kubegres.UID,
				// Controller intentionally left nil/false, simulating a PDB created
				// before owner references were set with NewControllerRef.
			}},
		},
		Spec: policy.PodDisruptionBudgetSpec{
			MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "database"}},
		},
	}
	if err := kubeClient.Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

	if err := enforcer.ensurePodDisruptionBudget(); err != nil {
		t.Fatal(err)
	}

	pdb, err := getPdb(t, kubeClient, "database")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdb.OwnerReferences) != 1 {
		t.Fatalf("expected exactly one owner reference, got %#v", pdb.OwnerReferences)
	}
	owner := pdb.OwnerReferences[0]
	if owner.Controller == nil || !*owner.Controller {
		t.Fatalf("expected owner reference to be repaired to Controller=true, got %#v", owner)
	}
}
