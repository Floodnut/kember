// Package controller contains Kember reconciliation logic.
package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

// TaskRunReconciler executes one immutable TaskRun snapshot through its resolved transport.
type TaskRunReconciler struct {
	client.Client
	APIReader     client.Reader
	Scheme        *runtime.Scheme
	Executor      PodExecutor
	WarmExecSlots chan struct{}
	Now           func() time.Time
	Metrics       *LifecycleMetrics
}

func (r *TaskRunReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	taskRun := &kemberv1.TaskRun{}
	if err := r.Get(ctx, request.NamespacedName, taskRun); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if taskRun.Status.Phase.IsTerminal() {
		if taskRun.Status.ResolvedTemplate != nil && taskRun.Status.ResolvedTemplate.LifecycleProfile == "warmLease" {
			return ctrl.Result{}, r.cleanupWarmWorker(ctx, taskRun)
		}
		return ctrl.Result{}, nil
	}
	if taskRun.Status.ResolvedTemplate == nil {
		if taskRun.Spec.Cancel {
			return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunCancelled, "TaskCancelled", "cancelled before Job creation")
		}
		return r.resolveTemplate(ctx, taskRun)
	}
	if taskRun.Status.DispatchedAt == nil && taskRun.Status.JobRef == nil && taskRun.Status.WorkerRef == nil {
		if taskRun.Spec.Cancel {
			return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunCancelled, "TaskCancelled", "cancelled before assignment")
		}
		if r.queueDeadlineExceeded(taskRun) {
			return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunTimedOut, "QueueTimedOut", "worker assignment exceeded WorkerPool queue timeout")
		}
	}
	if taskRun.Status.ResolvedTemplate.LifecycleProfile == "warmLease" {
		return r.reconcileWarmLease(ctx, taskRun)
	}

	job := &batchv1.Job{}
	err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: taskRun.Namespace, Name: jobName(taskRun)}, job)
	if apierrors.IsNotFound(err) {
		if taskRun.Status.JobRef != nil {
			return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunFailed, "JobMissing", "owned Job disappeared before terminal state")
		}
		if taskRun.Spec.Cancel {
			return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunCancelled, "TaskCancelled", "cancelled before Job creation")
		}
		return r.createJob(ctx, taskRun)
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if isJobComplete(job) {
		return ctrl.Result{}, r.finishFromJob(ctx, taskRun, job)
	}
	if taskRun.Spec.Cancel {
		return ctrl.Result{}, r.stopJob(ctx, taskRun, job, kemberv1.TaskRunCancelled, "TaskCancelled")
	}
	if taskRun.Status.DispatchedAt != nil && r.now().After(taskRun.Status.DispatchedAt.Add(time.Duration(taskRun.Spec.TimeoutSeconds)*time.Second)) {
		return ctrl.Result{}, r.stopJob(ctx, taskRun, job, kemberv1.TaskRunTimedOut, "TaskTimedOut")
	}
	if taskRun.Status.Phase != kemberv1.TaskRunRunning {
		taskRun.Status.Phase = kemberv1.TaskRunRunning
		return ctrl.Result{}, r.Status().Update(ctx, taskRun)
	}
	return ctrl.Result{RequeueAfter: time.Second}, nil
}

func (r *TaskRunReconciler) resolveTemplate(ctx context.Context, taskRun *kemberv1.TaskRun) (ctrl.Result, error) {
	pool := &kemberv1.WorkerPool{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: taskRun.Namespace, Name: taskRun.Spec.WorkerPoolRef.Name}, pool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunRejected, "WorkerPoolNotFound", "referenced WorkerPool does not exist")
		}
		return ctrl.Result{}, err
	}
	template, err := resolveTemplate(pool, taskRun)
	if err != nil {
		return ctrl.Result{}, r.finish(ctx, taskRun, kemberv1.TaskRunRejected, "TaskRejected", err.Error())
	}
	taskRun.Status.ResolvedTemplate = template
	taskRun.Status.Phase = kemberv1.TaskRunPending
	return ctrl.Result{Requeue: true}, r.Status().Update(ctx, taskRun)
}

func (r *TaskRunReconciler) createJob(ctx context.Context, taskRun *kemberv1.TaskRun) (ctrl.Result, error) {
	job := jobFor(taskRun)
	if err := ctrl.SetControllerReference(taskRun, job, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	now := kemberv1.NewPreciseTime(r.now())
	taskRun.Status.JobRef = &kemberv1.JobReference{Name: job.Name, UID: string(job.UID)}
	taskRun.Status.DispatchedAt = &now
	taskRun.Status.Phase = kemberv1.TaskRunRunning
	return ctrl.Result{}, r.Status().Update(ctx, taskRun)
}

// stopJob uses an uncached live read followed by a UID/resourceVersion preconditioned delete.
func (r *TaskRunReconciler) stopJob(ctx context.Context, taskRun *kemberv1.TaskRun, _ *batchv1.Job, phase kemberv1.TaskRunPhase, reason string) error {
	live := &batchv1.Job{}
	key := types.NamespacedName{Namespace: taskRun.Namespace, Name: jobName(taskRun)}
	if err := r.APIReader.Get(ctx, key, live); err != nil {
		if apierrors.IsNotFound(err) {
			return r.finish(ctx, taskRun, kemberv1.TaskRunFailed, "JobMissing", "owned Job disappeared during stop")
		}
		return err
	}
	if isJobComplete(live) {
		return r.finishFromJob(ctx, taskRun, live)
	}
	if err := r.Delete(ctx, live, &client.DeleteOptions{Raw: &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &live.UID, ResourceVersion: &live.ResourceVersion}}}); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			return fmt.Errorf("Job changed during conditional delete: %w", err)
		}
		return err
	}
	return r.finish(ctx, taskRun, phase, reason, strings.ToLower(string(phase)))
}

func (r *TaskRunReconciler) finishFromJob(ctx context.Context, taskRun *kemberv1.TaskRun, job *batchv1.Job) error {
	if jobSucceeded(job) {
		exitCode := int32(0)
		taskRun.Status.ExitCode = &exitCode
		taskRun.Status.Result = &kemberv1.TaskResult{Summary: "Job completed successfully"}
		return r.finish(ctx, taskRun, kemberv1.TaskRunSucceeded, "JobSucceeded", "Job completed successfully")
	}
	return r.finish(ctx, taskRun, kemberv1.TaskRunFailed, "WorkloadFailed", "Job failed")
}

func (r *TaskRunReconciler) finish(ctx context.Context, taskRun *kemberv1.TaskRun, phase kemberv1.TaskRunPhase, reason, message string) error {
	if taskRun.Status.Phase.IsTerminal() {
		return nil
	}
	now := kemberv1.NewPreciseTime(r.now())
	taskRun.Status.Phase = phase
	taskRun.Status.CompletedAt = &now
	taskRun.Status.Conditions = []metav1.Condition{{Type: "Completed", Status: metav1.ConditionTrue, Reason: reason, Message: message, LastTransitionTime: metav1.NewTime(now.Time)}}
	if err := r.Status().Update(ctx, taskRun); err != nil {
		return err
	}
	r.Metrics.ObserveTaskRunTerminal(taskRun, reason)
	return nil
}

func (r *TaskRunReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r *TaskRunReconciler) queueDeadlineExceeded(taskRun *kemberv1.TaskRun) bool {
	if taskRun.Status.DispatchedAt != nil || taskRun.Status.JobRef != nil || taskRun.Status.WorkerRef != nil || taskRun.Status.ResolvedTemplate == nil || taskRun.Status.ResolvedTemplate.QueueTimeoutSeconds < 1 || taskRun.CreationTimestamp.IsZero() {
		return false
	}
	deadline := taskRun.CreationTimestamp.Add(time.Duration(taskRun.Status.ResolvedTemplate.QueueTimeoutSeconds) * time.Second)
	return !r.now().Before(deadline)
}

func resolveTemplate(pool *kemberv1.WorkerPool, taskRun *kemberv1.TaskRun) (*kemberv1.ResolvedTemplate, error) {
	lifecycleProfile := "runToCompletion"
	if pool.Spec.Lifecycle != nil {
		lifecycleProfile = pool.Spec.Lifecycle.Profile
	}
	if pool.Spec.Execution.Mode == "job" {
		if lifecycleProfile != "runToCompletion" {
			return nil, fmt.Errorf("job execution requires runToCompletion lifecycle")
		}
	} else if pool.Spec.Execution.Mode == "exec" {
		if lifecycleProfile != "warmLease" || pool.Spec.Lifecycle == nil || pool.Spec.Lifecycle.MaxTasksPerWorker != 1 {
			return nil, fmt.Errorf("exec execution requires warmLease lifecycle with maxTasksPerWorker 1")
		}
		if pool.Spec.Capacity == nil || pool.Spec.Capacity.Policy != "fixed" || pool.Spec.Capacity.Size < 1 {
			return nil, fmt.Errorf("warmLease requires valid fixed capacity")
		}
		if len(pool.Spec.Execution.CommandTemplate) == 0 {
			return nil, fmt.Errorf("warmLease exec commandTemplate must not be empty")
		}
	} else {
		return nil, fmt.Errorf("unsupported WorkerPool execution mode")
	}
	if taskRun.Spec.TimeoutSeconds > pool.Spec.TaskPolicy.TimeoutSeconds {
		return nil, fmt.Errorf("timeoutSeconds exceeds WorkerPool policy")
	}
	if err := validateInput(taskRun.Spec.Input.Ref, pool.Spec.Template.InputPolicy.AllowedPrefixes); err != nil {
		return nil, err
	}
	for key, value := range taskRun.Spec.Parameters {
		schema, ok := pool.Spec.Template.AllowedParameters[key]
		if !ok || schema.Type != "string" {
			return nil, fmt.Errorf("parameter %q is not allowed", key)
		}
		if len(schema.Enum) > 0 && !contains(schema.Enum, value) {
			return nil, fmt.Errorf("parameter %q is outside its allowed values", key)
		}
	}
	args := append([]string(nil), pool.Spec.Template.Args...)
	execCommand := []string(nil)
	if pool.Spec.Execution.Mode == "job" {
		var err error
		args, err = renderArgs(pool.Spec.Template.ArgsTemplate, taskRun)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		execCommand, err = renderArgs(pool.Spec.Execution.CommandTemplate, taskRun)
		if err != nil {
			return nil, err
		}
	}
	resources := *pool.Spec.Template.Resources.DeepCopy()
	var securityContext = pool.Spec.Template.SecurityContext.DeepCopy()
	resolved := &kemberv1.ResolvedTemplate{WorkerPoolUID: string(pool.UID), WorkerPoolGeneration: pool.Generation, Image: pool.Spec.Template.Image, Command: append([]string(nil), pool.Spec.Template.Command...), Args: args, ExecutionMode: pool.Spec.Execution.Mode, LifecycleProfile: lifecycleProfile, ExecCommand: execCommand, ServiceAccountName: pool.Spec.Template.ServiceAccountName, Resources: resources, SecurityContext: securityContext, ActiveDeadlineSeconds: taskRun.Spec.TimeoutSeconds, QueueTimeoutSeconds: pool.Spec.TaskPolicy.QueueTimeoutSeconds, TTLSecondsAfterFinished: pool.Spec.TaskPolicy.RetentionSeconds}
	data, _ := json.Marshal(resolved)
	digest := sha256.Sum256(data)
	resolved.TemplateHash = "sha256:" + hex.EncodeToString(digest[:])
	return resolved, nil
}

func validateInput(raw string, prefixes []string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Contains(u.EscapedPath(), "%2f") || strings.Contains(u.EscapedPath(), "%2F") || strings.Contains(u.Path, "/../") || strings.HasSuffix(u.Path, "/..") || strings.Contains(u.Path, "/./") {
		return fmt.Errorf("input.ref is not a canonical absolute URI")
	}
	canonical := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.EscapedPath()
	for _, prefix := range prefixes {
		if strings.HasPrefix(canonical, prefix) {
			return nil
		}
	}
	return fmt.Errorf("input.ref is outside WorkerPool allowedPrefixes")
}

func renderArgs(templates []string, taskRun *kemberv1.TaskRun) ([]string, error) {
	args := make([]string, 0, len(templates))
	for _, arg := range templates {
		arg = strings.ReplaceAll(arg, "{{input.ref}}", taskRun.Spec.Input.Ref)
		for key, value := range taskRun.Spec.Parameters {
			arg = strings.ReplaceAll(arg, "{{params."+key+"}}", value)
		}
		if strings.Contains(arg, "{{") {
			return nil, fmt.Errorf("unresolved template placeholder")
		}
		args = append(args, arg)
	}
	return args, nil
}

func jobFor(taskRun *kemberv1.TaskRun) *batchv1.Job {
	t := taskRun.Status.ResolvedTemplate
	backoff := int32(0)
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName(taskRun), Namespace: taskRun.Namespace, Labels: map[string]string{"kember.openflood.org/taskrun-uid": string(taskRun.UID), "kember.openflood.org/workerpool": taskRun.Spec.WorkerPoolRef.Name}}, Spec: batchv1.JobSpec{BackoffLimit: &backoff, ActiveDeadlineSeconds: &t.ActiveDeadlineSeconds, TTLSecondsAfterFinished: &t.TTLSecondsAfterFinished, Template: corePodTemplate(t)}}
}

func corePodTemplate(t *kemberv1.ResolvedTemplate) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: t.ServiceAccountName,
			Containers: []corev1.Container{{
				Name:            "worker",
				Image:           t.Image,
				Command:         append([]string(nil), t.Command...),
				Args:            append([]string(nil), t.Args...),
				Resources:       *t.Resources.DeepCopy(),
				SecurityContext: t.SecurityContext.DeepCopy(),
			}},
		},
	}
}

func jobName(taskRun *kemberv1.TaskRun) string {
	return "kember-" + strings.ToLower(string(taskRun.UID))[:12]
}
func isJobComplete(job *batchv1.Job) bool { return jobSucceeded(job) || jobFailed(job) }
func jobSucceeded(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == "True" {
			return true
		}
	}
	return false
}
func jobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == "True" {
			return true
		}
	}
	return false
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
