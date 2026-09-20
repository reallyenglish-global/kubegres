package statefulset_spec

import (
	"context"
	"testing"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"reactive-tech.io/kubegres/internal/controller/ctx/log"
	"reactive-tech.io/kubegres/internal/controller/ctx/status"
	operation2 "reactive-tech.io/kubegres/internal/controller/operation"
	"reactive-tech.io/kubegres/internal/controller/states"
	statefulset2 "reactive-tech.io/kubegres/internal/controller/states/statefulset"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	minorUpgradeOldImage = "postgres:16.3"
	minorUpgradeNewImage = "postgres:16.4"
)

// newMinorUpgradeStatefulSet builds a bare StatefulSet with a single
// "postgres" container running the given image, named the way Kubegres names
// its instances ("<kubegres name>-<instanceIndex>").
func newMinorUpgradeStatefulSet(name, image string) apps.StatefulSet {
	return apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name)},
		Spec: apps.StatefulSetSpec{
			Template: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{{Name: "postgres", Image: image}},
				},
			},
		},
	}
}

func newMinorUpgradeReplicaWrapper(name string, instanceIndex int32, image string, readyForFailover bool) statefulset2.StatefulSetWrapper {
	return statefulset2.StatefulSetWrapper{
		IsDeployed:    true,
		IsReady:       readyForFailover,
		InstanceIndex: instanceIndex,
		StatefulSet:   newMinorUpgradeStatefulSet(name, image),
		Pod:           statefulset2.PodWrapper{IsReady: readyForFailover, InstanceIndex: instanceIndex},
	}
}

// minorUpgradeFixture wires up an AllStatefulSetsSpecEnforcer together with a
// real *operation2.BlockingOperation and a fake Kubernetes client, the way
// ResourcesContext does in production (see
// internal/controller/ctx/resources/ResourcesContext.go). Tests populate
// resourcesStates via setResourcesStates and register whichever
// BlockingOperationConfigs the scenario needs on blockingOp directly.
type minorUpgradeFixture struct {
	enforcer   AllStatefulSetsSpecEnforcer
	blockingOp *operation2.BlockingOperation
	kubegres   *postgresV1.Kubegres
	fakeClient client.Client
}

func buildMinorUpgradeFixture(t *testing.T, desiredImage string, initialObjs ...client.Object) *minorUpgradeFixture {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := apps.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	kubegres := &postgresV1.Kubegres{
		ObjectMeta: metav1.ObjectMeta{Name: "pg1", Namespace: "default"},
		Spec:       postgresV1.KubegresSpec{Image: desiredImage},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialObjs...).Build()

	kubegresContext := ctx.KubegresContext{
		Kubegres: kubegres,
		Ctx:      context.Background(),
		Client:   fakeClient,
		Log: log.LogWrapper{
			Kubegres: kubegres,
			Recorder: record.NewFakeRecorder(100),
		},
		Status: &status.KubegresStatusWrapper{Kubegres: kubegres},
	}

	blockingOp := operation2.CreateBlockingOperation(kubegresContext)

	f := &minorUpgradeFixture{
		blockingOp: blockingOp,
		kubegres:   kubegres,
		fakeClient: fakeClient,
	}
	f.setResourcesStates(states.ResourcesStates{})
	return f
}

// setResourcesStates (re)builds the enforcer with the given resourcesStates,
// since AllStatefulSetsSpecEnforcer holds it by value.
func (f *minorUpgradeFixture) setResourcesStates(resourcesStates states.ResourcesStates) {
	kubegresContext := ctx.KubegresContext{
		Kubegres: f.kubegres,
		Ctx:      context.Background(),
		Client:   f.fakeClient,
		Log:      log.LogWrapper{Kubegres: f.kubegres, Recorder: record.NewFakeRecorder(100)},
		Status:   &status.KubegresStatusWrapper{Kubegres: f.kubegres},
	}
	f.enforcer = CreateAllStatefulSetsSpecEnforcer(kubegresContext, resourcesStates, f.blockingOp, StatefulSetsSpecsEnforcer{})
}

func getStatefulSet(t *testing.T, c client.Client, name string) apps.StatefulSet {
	t.Helper()
	var sts apps.StatefulSet
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, &sts); err != nil {
		t.Fatalf("failed to get StatefulSet %q: %v", name, err)
	}
	return sts
}

// ---------------------------------------------------------------------------
// enforcePostgresMinorUpgrade
// ---------------------------------------------------------------------------

// No replica or primary differs from the desired image, and there is no
// active upgrade operation: enforcePostgresMinorUpgrade must be a no-op so
// that the normal spec-enforcement loop in EnforceSpec() can proceed.
func TestEnforcePostgresMinorUpgrade_NoPendingUpgrade_NoOp(t *testing.T) {
	f := buildMinorUpgradeFixture(t, minorUpgradeNewImage)

	var replicas statefulset2.StatefulSetWrappers
	replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeNewImage, true))

	resourcesStates := states.ResourcesStates{}
	resourcesStates.StatefulSets.Primary = newMinorUpgradeReplicaWrapper("pg1-0", 0, minorUpgradeNewImage, true)
	resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
	f.setResourcesStates(resourcesStates)

	handled, err := f.enforcer.enforcePostgresMinorUpgrade()
	if err != nil {
		t.Fatalf("enforcePostgresMinorUpgrade() error = %v, want nil", err)
	}
	if handled {
		t.Fatal("enforcePostgresMinorUpgrade() reported it handled the reconciliation, want no-op (false)")
	}
}

// A pending image change exists (a replica still runs the old image) but a
// *different* blocking operation is currently active: the upgrade must defer
// to it rather than starting or progressing.
func TestEnforcePostgresMinorUpgrade_DifferentBlockingOperationActive_Deferred(t *testing.T) {
	replicaSts := newMinorUpgradeStatefulSet("pg1-1", minorUpgradeOldImage)
	f := buildMinorUpgradeFixture(t, minorUpgradeNewImage, &replicaSts)

	var replicas statefulset2.StatefulSetWrappers
	replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeOldImage, true))

	resourcesStates := states.ResourcesStates{}
	resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
	f.setResourcesStates(resourcesStates)

	f.blockingOp.AddConfig(f.enforcer.CreateOperationConfigForStatefulSetSpecUpdating())
	if err := f.blockingOp.ActivateOperationOnStatefulSetSpecUpdate(
		operation2.OperationIdStatefulSetSpecEnforcing,
		operation2.OperationStepIdStatefulSetSpecUpdating,
		0, "SomeOtherSpec: changed"); err != nil {
		t.Fatalf("failed to activate unrelated blocking operation: %v", err)
	}

	handled, err := f.enforcer.enforcePostgresMinorUpgrade()
	if err != nil {
		t.Fatalf("enforcePostgresMinorUpgrade() error = %v, want nil", err)
	}
	if !handled {
		t.Fatal("enforcePostgresMinorUpgrade() = false, want true (deferred to the other active operation)")
	}

	// The upgrade must not have been started while blocked.
	active := f.blockingOp.GetActiveOperation()
	if active.OperationId != operation2.OperationIdStatefulSetSpecEnforcing {
		t.Fatalf("the unrelated operation must remain active, got OperationId=%q", active.OperationId)
	}

	sts := getStatefulSet(t, f.fakeClient, "pg1-1")
	if sts.Spec.Template.Spec.Containers[0].Image != minorUpgradeOldImage {
		t.Fatal("the replica's image must not be touched while a different blocking operation is active")
	}
}

// ---------------------------------------------------------------------------
// startPostgresMinorUpgrade
// ---------------------------------------------------------------------------

// The one out-of-date replica is not yet safe to touch (not ready for
// failover): startPostgresMinorUpgrade must return early without activating
// any operation or mutating the replica.
func TestStartPostgresMinorUpgrade_ReplicaNotReady_EarlyReturn(t *testing.T) {
	replicaSts := newMinorUpgradeStatefulSet("pg1-1", minorUpgradeOldImage)
	f := buildMinorUpgradeFixture(t, minorUpgradeNewImage, &replicaSts)

	var replicas statefulset2.StatefulSetWrappers
	replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeOldImage, false /* not ready for failover */))
	resourcesStates := states.ResourcesStates{}
	resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
	f.setResourcesStates(resourcesStates)

	if err := f.enforcer.startPostgresMinorUpgrade(minorUpgradeNewImage); err != nil {
		t.Fatalf("startPostgresMinorUpgrade() error = %v, want nil", err)
	}

	if active := f.blockingOp.GetActiveOperation(); active.OperationId != "" {
		t.Fatalf("no operation should have been activated for a not-ready replica, got %+v", active)
	}

	sts := getStatefulSet(t, f.fakeClient, "pg1-1")
	if sts.Spec.Template.Spec.Containers[0].Image != minorUpgradeOldImage {
		t.Fatal("a not-ready replica's image must not be changed")
	}
}

// The one out-of-date replica is ready for failover: startPostgresMinorUpgrade
// must activate the "replica is updating" operation step and push the new
// image to the replica's StatefulSet.
func TestStartPostgresMinorUpgrade_ReplicaReady_ActivatesUpgrade(t *testing.T) {
	replicaSts := newMinorUpgradeStatefulSet("pg1-1", minorUpgradeOldImage)
	f := buildMinorUpgradeFixture(t, minorUpgradeNewImage, &replicaSts)

	var replicas statefulset2.StatefulSetWrappers
	replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeOldImage, true /* ready for failover */))
	resourcesStates := states.ResourcesStates{}
	resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
	f.setResourcesStates(resourcesStates)

	f.blockingOp.AddConfig(f.enforcer.CreateOperationConfigForPostgresMinorUpgradeReplica())

	if err := f.enforcer.startPostgresMinorUpgrade(minorUpgradeNewImage); err != nil {
		t.Fatalf("startPostgresMinorUpgrade() error = %v, want nil", err)
	}

	active := f.blockingOp.GetActiveOperation()
	if active.OperationId != operation2.OperationIdPostgresMinorVersionUpgrade || active.StepId != operation2.OperationStepIdPostgresUpgradeReplica {
		t.Fatalf("unexpected active operation: %+v", active)
	}
	if active.StatefulSetOperation.InstanceIndex != 1 {
		t.Fatalf("active operation targets instance %d, want 1", active.StatefulSetOperation.InstanceIndex)
	}

	sts := getStatefulSet(t, f.fakeClient, "pg1-1")
	if sts.Spec.Template.Spec.Containers[0].Image != minorUpgradeNewImage {
		t.Fatalf("replica image = %q, want %q", sts.Spec.Template.Spec.Containers[0].Image, minorUpgradeNewImage)
	}
}

// ---------------------------------------------------------------------------
// isPostgresMinorUpgradeReplicaReady / hasPendingPostgresUpgrade
// ---------------------------------------------------------------------------

func TestIsPostgresMinorUpgradeReplicaReady(t *testing.T) {
	tests := map[string]struct {
		image  string
		ready  bool
		index  int32
		lookup int32
		want   bool
	}{
		"replica missing":            {image: minorUpgradeNewImage, ready: true, index: 1, lookup: 2, want: false},
		"image not yet upgraded":     {image: minorUpgradeOldImage, ready: true, index: 1, lookup: 1, want: false},
		"upgraded but not yet ready": {image: minorUpgradeNewImage, ready: false, index: 1, lookup: 1, want: false},
		"upgraded and ready":         {image: minorUpgradeNewImage, ready: true, index: 1, lookup: 1, want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := buildMinorUpgradeFixture(t, minorUpgradeNewImage)
			var replicas statefulset2.StatefulSetWrappers
			replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", tc.index, tc.image, tc.ready))
			resourcesStates := states.ResourcesStates{}
			resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
			f.setResourcesStates(resourcesStates)

			op := postgresV1.KubegresBlockingOperation{
				StatefulSetOperation: postgresV1.KubegresStatefulSetOperation{InstanceIndex: tc.lookup},
			}
			if got := f.enforcer.isPostgresMinorUpgradeReplicaReady(op); got != tc.want {
				t.Fatalf("isPostgresMinorUpgradeReplicaReady() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasPendingPostgresUpgrade(t *testing.T) {
	t.Run("no active operation and all replicas up to date", func(t *testing.T) {
		f := buildMinorUpgradeFixture(t, minorUpgradeNewImage)
		var replicas statefulset2.StatefulSetWrappers
		replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeNewImage, true))
		resourcesStates := states.ResourcesStates{}
		resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
		f.setResourcesStates(resourcesStates)

		if f.enforcer.hasPendingPostgresUpgrade(minorUpgradeNewImage) {
			t.Fatal("hasPendingPostgresUpgrade() = true, want false")
		}
	})

	t.Run("a replica still runs the old image", func(t *testing.T) {
		f := buildMinorUpgradeFixture(t, minorUpgradeNewImage)
		var replicas statefulset2.StatefulSetWrappers
		replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeOldImage, true))
		resourcesStates := states.ResourcesStates{}
		resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
		f.setResourcesStates(resourcesStates)

		if !f.enforcer.hasPendingPostgresUpgrade(minorUpgradeNewImage) {
			t.Fatal("hasPendingPostgresUpgrade() = false, want true")
		}
	})

	t.Run("upgrade operation already active even though images now match", func(t *testing.T) {
		f := buildMinorUpgradeFixture(t, minorUpgradeNewImage)
		var replicas statefulset2.StatefulSetWrappers
		replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeNewImage, true))
		resourcesStates := states.ResourcesStates{}
		resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
		f.setResourcesStates(resourcesStates)

		f.blockingOp.AddConfig(f.enforcer.CreateOperationConfigForPostgresMinorUpgradeReplica())
		if err := f.blockingOp.ActivateOperationOnStatefulSet(operation2.OperationIdPostgresMinorVersionUpgrade, operation2.OperationStepIdPostgresUpgradeReplica, 1); err != nil {
			t.Fatalf("failed to activate operation: %v", err)
		}

		if !f.enforcer.hasPendingPostgresUpgrade(minorUpgradeNewImage) {
			t.Fatal("hasPendingPostgresUpgrade() = false, want true while the upgrade operation is still active")
		}
	})
}

// ---------------------------------------------------------------------------
// Full state-machine transition: replica becomes ready -> failover begins
// ---------------------------------------------------------------------------

// Once the upgraded replica becomes ready, BlockingOperation's own completion
// checker (isPostgresMinorUpgradeReplicaOperationComplete) moves the "replica
// is updating" step into transition, and enforcePostgresMinorUpgrade must
// then begin the failover by deleting the old primary and activating the
// "waiting before failover" step (PrimaryToReplicaFailOver.
// BeginPostgresMinorUpgradeFailover). This exercises the switch-case for
// OperationStepIdPostgresUpgradeReplica in enforcePostgresMinorUpgrade.
func TestEnforcePostgresMinorUpgrade_ReplicaBecomesReady_TransitionsToWaitingBeforeFailover(t *testing.T) {
	primarySts := newMinorUpgradeStatefulSet("pg1-0", minorUpgradeNewImage)
	f := buildMinorUpgradeFixture(t, minorUpgradeNewImage, &primarySts)

	var replicas statefulset2.StatefulSetWrappers
	// The replica has already been upgraded and is now healthy again.
	replicas.Add(newMinorUpgradeReplicaWrapper("pg1-1", 1, minorUpgradeNewImage, true))

	resourcesStates := states.ResourcesStates{}
	resourcesStates.StatefulSets.Primary = newMinorUpgradeReplicaWrapper("pg1-0", 0, minorUpgradeNewImage, true)
	resourcesStates.StatefulSets.Replicas = statefulset2.Replicas{All: replicas}
	f.setResourcesStates(resourcesStates)

	f.blockingOp.AddConfig(f.enforcer.CreateOperationConfigForPostgresMinorUpgradeReplica())
	f.blockingOp.AddConfig(f.enforcer.CreateOperationConfigForPostgresMinorUpgradeWaiting())

	// Simulate that startPostgresMinorUpgrade already ran in an earlier
	// reconciliation and put replica instance 1 into the "updating" step.
	if err := f.blockingOp.ActivateOperationOnStatefulSet(
		operation2.OperationIdPostgresMinorVersionUpgrade,
		operation2.OperationStepIdPostgresUpgradeReplica,
		1); err != nil {
		t.Fatalf("failed to activate the replica-updating operation: %v", err)
	}

	// Re-derive the active/previous operation from status, which runs the
	// registered CompletionChecker (isPostgresMinorUpgradeReplicaOperationComplete)
	// and, since the replica is now upgraded and ready, moves the operation
	// into its transition step.
	f.blockingOp.LoadActiveOperation()

	if !f.blockingOp.IsActiveOperationInTransition(operation2.OperationIdPostgresMinorVersionUpgrade) {
		t.Fatalf("test setup failed: expected the operation to be in transition, got %+v", f.blockingOp.GetActiveOperation())
	}

	handled, err := f.enforcer.enforcePostgresMinorUpgrade()
	if err != nil {
		t.Fatalf("enforcePostgresMinorUpgrade() error = %v, want nil", err)
	}
	if !handled {
		t.Fatal("enforcePostgresMinorUpgrade() = false, want true")
	}

	active := f.blockingOp.GetActiveOperation()
	if active.StepId != operation2.OperationStepIdPostgresUpgradeWaitingBeforeFailover {
		t.Fatalf("active operation step = %q, want %q", active.StepId, operation2.OperationStepIdPostgresUpgradeWaitingBeforeFailover)
	}
	if active.StatefulSetOperation.InstanceIndex != 1 {
		t.Fatalf("the promoted replica's instance index = %d, want 1", active.StatefulSetOperation.InstanceIndex)
	}

	// The old primary must have been removed so the replica-count enforcer
	// can later recreate it as a fresh replica of the new primary.
	if err := f.fakeClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "pg1-0"}, &apps.StatefulSet{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected the old primary StatefulSet to be deleted, Get() error = %v", err)
	}
}
