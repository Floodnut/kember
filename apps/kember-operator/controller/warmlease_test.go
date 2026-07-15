package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

func TestResolveTemplateSnapshotsWarmLeaseCommandsSeparately(t *testing.T) {
	pool := testWarmLeasePool()
	taskRun := &kemberv1.TaskRun{Spec: kemberv1.TaskRunSpec{
		Parameters:     map[string]string{"severity": "HIGH"},
		Input:          kemberv1.TaskInput{Ref: "s3://security-artifacts/project-a/source.tar.gz"},
		TimeoutSeconds: 60,
	}}

	resolved, err := resolveTemplate(pool, taskRun)
	if err != nil {
		t.Fatalf("resolveTemplate() error = %v", err)
	}
	if resolved.LifecycleProfile != "warmLease" || resolved.ExecutionMode != "exec" {
		t.Fatalf("resolved mode = %s/%s, want warmLease/exec", resolved.LifecycleProfile, resolved.ExecutionMode)
	}
	if got, want := resolved.Args, []string{"--idle"}; !sameStrings(got, want) {
		t.Fatalf("bootstrap args = %v, want %v", got, want)
	}
	if got, want := resolved.ExecCommand, []string{"/app/run", "--severity", "HIGH", "s3://security-artifacts/project-a/source.tar.gz"}; !sameStrings(got, want) {
		t.Fatalf("exec command = %v, want %v", got, want)
	}
}

func TestWarmLeaseAlphaRejectsReuse(t *testing.T) {
	pool := testWarmLeasePool()
	pool.Spec.Lifecycle.MaxTasksPerWorker = 2
	if err := validateWarmLeasePool(pool); err == nil {
		t.Fatal("validateWarmLeasePool() accepted maxTasksPerWorker > 1")
	}
}

func TestTryAcquireExecSlotDoesNotBlockWhenFull(t *testing.T) {
	slots := make(chan struct{}, 1)
	if !tryAcquireExecSlot(slots) {
		t.Fatal("first exec slot acquisition failed")
	}
	if tryAcquireExecSlot(slots) {
		t.Fatal("full exec slot channel accepted another acquisition")
	}
	<-slots
	if !tryAcquireExecSlot(slots) {
		t.Fatal("released exec slot was not reusable")
	}
}

func TestWorkerStillAvailableUsesLiveUID(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	expected := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "tasks", UID: types.UID("worker-uid")}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(expected.DeepCopy()).Build()
	reconciler := &TaskRunReconciler{Client: fakeClient, APIReader: fakeClient}

	available, err := reconciler.workerStillAvailable(context.Background(), expected)
	if err != nil || !available {
		t.Fatalf("workerStillAvailable() = %t, %v, want true", available, err)
	}
	if err := fakeClient.Delete(context.Background(), expected.DeepCopy()); err != nil {
		t.Fatal(err)
	}
	available, err = reconciler.workerStillAvailable(context.Background(), expected)
	if err != nil || available {
		t.Fatalf("workerStillAvailable() after delete = %t, %v, want false", available, err)
	}
}

func TestWarmWorkerPodUsesBootstrapAndGenerationIdentity(t *testing.T) {
	pool := testWarmLeasePool()
	pod := warmWorkerPod(pool)
	if got, want := pod.Spec.Containers[0].Command, []string{"/app/bootstrap"}; !sameStrings(got, want) {
		t.Fatalf("worker command = %v, want %v", got, want)
	}
	if got := pod.Labels[workerPoolGenerationLabel]; got != "3" {
		t.Fatalf("generation label = %q, want 3", got)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyAlways || pod.Spec.Containers[0].ReadinessProbe == nil {
		t.Fatal("warm worker must restart and carry readiness probe")
	}
}

func testWarmLeasePool() *kemberv1.WorkerPool {
	pool := testWorkerPool()
	pool.Spec.Execution = kemberv1.ExecutionSpec{Mode: "exec", CommandTemplate: []string{"/app/run", "--severity", "{{params.severity}}", "{{input.ref}}"}}
	pool.Spec.Lifecycle = &kemberv1.LifecycleSpec{Profile: "warmLease", MaxTasksPerWorker: 1}
	pool.Spec.Capacity = &kemberv1.CapacitySpec{Policy: "fixed", Size: 2}
	pool.Spec.Template.Command = []string{"/app/bootstrap"}
	pool.Spec.Template.Args = []string{"--idle"}
	pool.Spec.Template.ReadinessProbe = &corev1.Probe{InitialDelaySeconds: 1}
	pool.ObjectMeta = metav1.ObjectMeta{Name: "scanner", Namespace: "security", UID: pool.UID, Generation: pool.Generation}
	return pool
}
