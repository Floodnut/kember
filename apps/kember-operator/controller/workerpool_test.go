package controller

import (
	"context"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
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

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool, pod, lease).Build()
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
}
