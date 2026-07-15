package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

const (
	workerPoolLabel           = "kember.dev/workerpool"
	workerPoolUIDLabel        = "kember.dev/workerpool-uid"
	workerPoolGenerationLabel = "kember.dev/workerpool-generation"
	workerTaskRunLabel        = "kember.dev/taskrun-uid"
	workerContainerName       = "worker"
)

// WorkerPoolReconciler maintains the fixed non-terminating worker count of WarmLease pools.
type WorkerPoolReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
}

func (r *WorkerPoolReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	pool := &kemberv1.WorkerPool{}
	if err := r.Get(ctx, request.NamespacedName, pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !isWarmLeasePool(pool) {
		return ctrl.Result{}, nil
	}
	if err := validateWarmLeasePool(pool); err != nil {
		return ctrl.Result{}, err
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(pool.Namespace), client.MatchingLabels{workerPoolUIDLabel: string(pool.UID)}); err != nil {
		return ctrl.Result{}, err
	}
	managed := 0
	available := make([]*corev1.Pod, 0, len(pods.Items))
	for i := range pods.Items {
		pod := &pods.Items[i]
		leased, err := r.podHasLease(ctx, pod)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !pod.DeletionTimestamp.IsZero() {
			continue
		}
		if pod.Labels[workerPoolGenerationLabel] != strconv.FormatInt(pool.Generation, 10) || pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			if !leased && pod.DeletionTimestamp.IsZero() {
				if err := r.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			} else if leased {
				managed++
			}
			continue
		}
		managed++
		if !leased {
			available = append(available, pod)
		}
	}

	size := int(pool.Spec.Capacity.Size)
	for managed > size && len(available) > 0 {
		pod := available[len(available)-1]
		available = available[:len(available)-1]
		if err := r.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		managed--
	}
	if managed < size {
		pod := warmWorkerPod(pool)
		if err := ctrl.SetControllerReference(pool, pod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *WorkerPoolReconciler) podHasLease(ctx context.Context, pod *corev1.Pod) (bool, error) {
	lease := &coordinationv1.Lease{}
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	err := reader.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: leaseNameForPod(pod.Name)}, lease)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func isWarmLeasePool(pool *kemberv1.WorkerPool) bool {
	return pool.Spec.Lifecycle != nil && pool.Spec.Lifecycle.Profile == "warmLease"
}

func validateWarmLeasePool(pool *kemberv1.WorkerPool) error {
	if pool.Spec.Execution.Mode != "exec" || len(pool.Spec.Execution.CommandTemplate) == 0 {
		return fmt.Errorf("warmLease WorkerPool requires exec commandTemplate")
	}
	if pool.Spec.Lifecycle.MaxTasksPerWorker != 1 {
		return fmt.Errorf("warmLease alpha supports maxTasksPerWorker 1 only")
	}
	if pool.Spec.Capacity == nil || pool.Spec.Capacity.Policy != "fixed" || pool.Spec.Capacity.Size < 1 {
		return fmt.Errorf("warmLease alpha requires fixed capacity with size at least 1")
	}
	return nil
}

func warmWorkerPod(pool *kemberv1.WorkerPool) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: pool.Name + "-",
			Namespace:    pool.Namespace,
			Labels: map[string]string{
				workerPoolLabel:           pool.Name,
				workerPoolUIDLabel:        string(pool.UID),
				workerPoolGenerationLabel: strconv.FormatInt(pool.Generation, 10),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyAlways,
			ServiceAccountName: pool.Spec.Template.ServiceAccountName,
			Containers: []corev1.Container{{
				Name:            workerContainerName,
				Image:           pool.Spec.Template.Image,
				Command:         append([]string(nil), pool.Spec.Template.Command...),
				Args:            append([]string(nil), pool.Spec.Template.Args...),
				Resources:       *pool.Spec.Template.Resources.DeepCopy(),
				SecurityContext: pool.Spec.Template.SecurityContext.DeepCopy(),
				ReadinessProbe:  pool.Spec.Template.ReadinessProbe.DeepCopy(),
			}},
		},
	}
}

func leaseNameForPod(podName string) string {
	return podName
}
