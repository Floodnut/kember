package controller

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

// PodExecutor executes an argv array in an already Ready worker container.
type PodExecutor interface {
	Execute(ctx context.Context, namespace, pod, container string, command []string) (int32, error)
}

func (r *TaskRunReconciler) reconcileWarmLease(ctx context.Context, taskRun *kemberv1.TaskRun) (ctrl.Result, error) {
	if taskRun.Status.WorkerRef != nil {
		if err := r.finish(ctx, taskRun, kemberv1.TaskRunFailed, "ExecutionOutcomeUnknown", "operator restarted after dispatch; worker was drained without replay"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, r.cleanupWarmWorker(ctx, taskRun)
	}
	if taskRun.Spec.Cancel {
		return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunCancelled, "TaskCancelled", "cancelled before worker assignment")
	}
	if r.Executor == nil {
		return ctrl.Result{}, fmt.Errorf("PodExecutor is not configured")
	}
	if r.WarmExecSlots == nil {
		return ctrl.Result{}, fmt.Errorf("WarmLease exec slots are not configured")
	}
	if tryAcquireExecSlot(r.WarmExecSlots) {
		defer func() { <-r.WarmExecSlots }()
	} else {
		return ctrl.Result{RequeueAfter: 250 * time.Millisecond}, nil
	}

	pod, lease, err := r.acquireWarmWorker(ctx, taskRun)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pod == nil {
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	now := metav1.NewTime(r.now())
	taskRun.Status.WorkerRef = &kemberv1.WorkerReference{Name: pod.Name, UID: string(pod.UID), LeaseName: lease.Name}
	taskRun.Status.DispatchedAt = &now
	taskRun.Status.Phase = kemberv1.TaskRunRunning
	if err := r.Status().Update(ctx, taskRun); err != nil {
		return ctrl.Result{}, err
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(taskRun.Spec.TimeoutSeconds)*time.Second)
	exitCode, execErr := r.Executor.Execute(execCtx, pod.Namespace, pod.Name, workerContainerName, taskRun.Status.ResolvedTemplate.ExecCommand)
	timedOut := execCtx.Err() == context.DeadlineExceeded
	cancel()
	taskRun.Status.ExitCode = &exitCode
	if timedOut {
		execErr = r.finish(ctx, taskRun, kemberv1.TaskRunTimedOut, "TaskTimedOut", "warm worker command exceeded timeout")
	} else if execErr != nil {
		execErr = r.finish(ctx, taskRun, kemberv1.TaskRunFailed, "WorkerExecutionError", execErr.Error())
	} else if exitCode != 0 {
		available, err := r.workerStillAvailable(ctx, pod)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !available {
			execErr = r.finish(ctx, taskRun, kemberv1.TaskRunFailed, "WorkerLost", fmt.Sprintf("worker disappeared while command exited with code %d", exitCode))
		} else {
			execErr = r.finish(ctx, taskRun, kemberv1.TaskRunFailed, "WorkloadFailed", fmt.Sprintf("warm worker command exited with code %d", exitCode))
		}
	} else {
		taskRun.Status.Result = &kemberv1.TaskResult{Summary: "Warm worker command completed successfully"}
		execErr = r.finish(ctx, taskRun, kemberv1.TaskRunSucceeded, "WorkerCommandSucceeded", "warm worker command completed successfully")
	}
	if execErr != nil {
		return ctrl.Result{}, execErr
	}
	return ctrl.Result{}, r.cleanupWarmWorker(ctx, taskRun)
}

func (r *TaskRunReconciler) workerStillAvailable(ctx context.Context, expected *corev1.Pod) (bool, error) {
	live := &corev1.Pod{}
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	err := reader.Get(ctx, types.NamespacedName{Namespace: expected.Namespace, Name: expected.Name}, live)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return live.UID == expected.UID && live.DeletionTimestamp.IsZero(), nil
}

func tryAcquireExecSlot(slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (r *TaskRunReconciler) acquireWarmWorker(ctx context.Context, taskRun *kemberv1.TaskRun) (*corev1.Pod, *coordinationv1.Lease, error) {
	t := taskRun.Status.ResolvedTemplate
	pods := &corev1.PodList{}
	labels := client.MatchingLabels{workerPoolUIDLabel: t.WorkerPoolUID, workerPoolGenerationLabel: strconv.FormatInt(t.WorkerPoolGeneration, 10)}
	if err := r.List(ctx, pods, client.InNamespace(taskRun.Namespace), labels); err != nil {
		return nil, nil, err
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !podReady(pod) || !pod.DeletionTimestamp.IsZero() {
			continue
		}
		lease := &coordinationv1.Lease{}
		key := types.NamespacedName{Namespace: pod.Namespace, Name: leaseNameForPod(pod.Name)}
		err := r.Get(ctx, key, lease)
		if err == nil {
			if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == string(taskRun.UID) {
				return pod, lease, nil
			}
			continue
		}
		if !apierrors.IsNotFound(err) {
			return nil, nil, err
		}

		holder := string(taskRun.UID)
		duration := int32(taskRun.Spec.TimeoutSeconds + 30)
		acquireTime := metav1.NewMicroTime(r.now())
		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, Labels: map[string]string{workerPoolUIDLabel: t.WorkerPoolUID, workerTaskRunLabel: holder}},
			Spec:       coordinationv1.LeaseSpec{HolderIdentity: &holder, LeaseDurationSeconds: &duration, AcquireTime: &acquireTime},
		}
		lease.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: pod.Name, UID: pod.UID}}
		if err := r.Create(ctx, lease); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return nil, nil, err
		}
		before := pod.DeepCopy()
		if pod.Labels == nil {
			pod.Labels = map[string]string{}
		}
		pod.Labels[workerTaskRunLabel] = holder
		if err := r.Patch(ctx, pod, client.MergeFrom(before)); err != nil {
			_ = r.Delete(ctx, lease)
			return nil, nil, err
		}
		return pod, lease, nil
	}
	return nil, nil, nil
}

func (r *TaskRunReconciler) cleanupWarmWorker(ctx context.Context, taskRun *kemberv1.TaskRun) error {
	if taskRun.Status.WorkerRef == nil {
		return nil
	}
	pod := &corev1.Pod{}
	key := types.NamespacedName{Namespace: taskRun.Namespace, Name: taskRun.Status.WorkerRef.Name}
	if err := r.APIReader.Get(ctx, key, pod); err != nil {
		return client.IgnoreNotFound(err)
	}
	if string(pod.UID) != taskRun.Status.WorkerRef.UID {
		return nil
	}
	return client.IgnoreNotFound(r.Delete(ctx, pod, &client.DeleteOptions{Raw: &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pod.UID}}}))
}

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
