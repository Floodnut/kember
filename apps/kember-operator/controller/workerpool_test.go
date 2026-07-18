package controller

import (
	"context"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

func TestWorkerPoolFixedSizeIncludesLeasedWorkers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := kemberv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	pool := testWarmLeasePool()
	pool.Spec.Capacity.Size = 1
	pod := warmWorkerPod(pool)
	pod.GenerateName = ""
	pod.Name = "scanner-worker"
	pod.UID = types.UID("worker-uid")
	holder := "taskrun-uid"
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: &holder},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&kemberv1.WorkerPool{}).WithObjects(pool, pod, lease).Build()
	reconciler := &WorkerPoolReconciler{Client: fakeClient, APIReader: fakeClient, Scheme: scheme}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	pods := &corev1.PodList{}
	if err := fakeClient.List(context.Background(), pods, client.InNamespace(pool.Namespace), client.MatchingLabels{workerPoolUIDLabel: string(pool.UID)}); err != nil {
		t.Fatal(err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("managed pods = %d, want fixed total size 1 while the worker is leased", len(pods.Items))
	}
	updated := &kemberv1.WorkerPool{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pool), updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Status.Capacity; got.Desired != 1 || got.Leased != 1 || got.Starting != 0 || got.Ready != 0 || got.Terminating != 0 {
		t.Fatalf("capacity status = %+v, want desired=1 leased=1", got)
	}
	if condition := apimeta.FindStatusCondition(updated.Status.Conditions, "Ready"); condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition = %+v, want True", condition)
	}

	storedPod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pod), storedPod); err != nil {
		t.Fatal(err)
	}
	storedPod.Labels[workerPoolGenerationLabel] = "2"
	if err := fakeClient.Update(context.Background(), storedPod); err != nil {
		t.Fatal(err)
	}
	observedPod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pod), observedPod); err != nil {
		t.Fatal(err)
	}
	if observedPod.Labels[workerPoolGenerationLabel] != "2" {
		t.Fatalf("worker generation label = %q, want 2", observedPod.Labels[workerPoolGenerationLabel])
	}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err != nil {
		t.Fatalf("Reconcile() with outdated leased worker error = %v", err)
	}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pool), updated); err != nil {
		t.Fatal(err)
	}
	if condition := apimeta.FindStatusCondition(updated.Status.Conditions, "Ready"); condition == nil || condition.Status != metav1.ConditionFalse {
		t.Fatalf("Ready condition with outdated leased worker = %+v, want False", condition)
	}
	if condition := apimeta.FindStatusCondition(updated.Status.Conditions, "Progressing"); condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatalf("Progressing condition with outdated leased worker = %+v, want True", condition)
	}
}

func TestWorkerPoolStatusReportsObservedCapacity(t *testing.T) {
	scheme := workerPoolTestScheme(t)
	pool := testWarmLeasePool()
	pool.Spec.Capacity.Size = 2

	ready := namedWorkerPod(pool, "ready-worker")
	ready.Status.Phase = corev1.PodRunning
	ready.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	starting := namedWorkerPod(pool, "starting-worker")
	starting.Status.Phase = corev1.PodPending

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&kemberv1.WorkerPool{}).WithObjects(pool, ready, starting).Build()
	reconciler := &WorkerPoolReconciler{Client: fakeClient, APIReader: fakeClient, Scheme: scheme}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &kemberv1.WorkerPool{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pool), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.ObservedGeneration != pool.Generation {
		t.Fatalf("observed generation = %d, want %d", updated.Status.ObservedGeneration, pool.Generation)
	}
	if got := updated.Status.Capacity; got.Desired != 2 || got.Ready != 1 || got.Starting != 1 || got.Leased != 0 || got.Terminating != 0 {
		t.Fatalf("capacity status = %+v, want desired=2 ready=1 starting=1", got)
	}
	if condition := apimeta.FindStatusCondition(updated.Status.Conditions, "Ready"); condition == nil || condition.Status != metav1.ConditionFalse {
		t.Fatalf("Ready condition = %+v, want False", condition)
	}
	if condition := apimeta.FindStatusCondition(updated.Status.Conditions, "Progressing"); condition == nil || condition.Status != metav1.ConditionTrue {
		t.Fatalf("Progressing condition = %+v, want True", condition)
	}
}

func TestWorkerPoolStatusReportsInvalidSpec(t *testing.T) {
	scheme := workerPoolTestScheme(t)
	pool := testWarmLeasePool()
	pool.Spec.Lifecycle.MaxTasksPerWorker = 2
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&kemberv1.WorkerPool{}).WithObjects(pool).Build()
	reconciler := &WorkerPoolReconciler{Client: fakeClient, APIReader: fakeClient, Scheme: scheme}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pool)}); err == nil {
		t.Fatal("Reconcile() accepted maxTasksPerWorker 2")
	}

	updated := &kemberv1.WorkerPool{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pool), updated); err != nil {
		t.Fatal(err)
	}
	condition := apimeta.FindStatusCondition(updated.Status.Conditions, "Degraded")
	if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != "InvalidSpec" {
		t.Fatalf("Degraded condition = %+v, want True/InvalidSpec", condition)
	}
}

func workerPoolTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, coordinationv1.AddToScheme, kemberv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return scheme
}

func namedWorkerPod(pool *kemberv1.WorkerPool, name string) *corev1.Pod {
	pod := warmWorkerPod(pool)
	pod.GenerateName = ""
	pod.Name = name
	pod.UID = types.UID(name + "-uid")
	return pod
}
