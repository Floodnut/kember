package controller

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

func TestResolveTemplateSnapshotsExecutionContract(t *testing.T) {
	pool := testWorkerPool()
	taskRun := &kemberv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{Namespace: "security", UID: types.UID("0123456789abcdef")},
		Spec: kemberv1.TaskRunSpec{
			Parameters:     map[string]string{"severity": "HIGH"},
			Input:          kemberv1.TaskInput{Ref: "s3://security-artifacts/project-a/source.tar.gz"},
			TimeoutSeconds: 60,
		},
	}

	resolved, err := resolveTemplate(pool, taskRun)
	if err != nil {
		t.Fatalf("resolveTemplate() error = %v", err)
	}
	if got, want := resolved.Args, []string{"--severity", "HIGH", "s3://security-artifacts/project-a/source.tar.gz"}; !sameStrings(got, want) {
		t.Fatalf("resolved args = %v, want %v", got, want)
	}
	if resolved.TemplateHash == "" || resolved.TemplateHash[:7] != "sha256:" {
		t.Fatalf("template hash = %q, want sha256 digest", resolved.TemplateHash)
	}
	if resolved.QueueTimeoutSeconds != 30 {
		t.Fatalf("queue timeout snapshot = %d, want 30", resolved.QueueTimeoutSeconds)
	}

	pool.Spec.Template.Command[0] = "mutated"
	if resolved.Command[0] != "/app/scan-worker" {
		t.Fatalf("resolved command was mutated: %v", resolved.Command)
	}
}

func TestQueueDeadlineExceededOnlyBeforeAssignment(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	taskRun := &kemberv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-31 * time.Second))},
		Status: kemberv1.TaskRunStatus{
			ResolvedTemplate: &kemberv1.ResolvedTemplate{QueueTimeoutSeconds: 30},
		},
	}
	reconciler := &TaskRunReconciler{Now: func() time.Time { return now }}
	if !reconciler.queueDeadlineExceeded(taskRun) {
		t.Fatal("queue deadline was not exceeded after 31 seconds")
	}
	dispatched := kemberv1.NewPreciseTime(now.Add(-time.Second))
	taskRun.Status.DispatchedAt = &dispatched
	if reconciler.queueDeadlineExceeded(taskRun) {
		t.Fatal("queue deadline applied after assignment")
	}
}

func TestResolveTemplateRejectsInputOutsidePolicy(t *testing.T) {
	taskRun := &kemberv1.TaskRun{Spec: kemberv1.TaskRunSpec{Input: kemberv1.TaskInput{Ref: "s3://other-bucket/source.tar.gz"}, TimeoutSeconds: 60}}
	if _, err := resolveTemplate(testWorkerPool(), taskRun); err == nil {
		t.Fatal("resolveTemplate() succeeded for input outside allowed prefixes")
	}
}

func TestResolveTemplateRejectsUnknownParameter(t *testing.T) {
	taskRun := &kemberv1.TaskRun{Spec: kemberv1.TaskRunSpec{Parameters: map[string]string{"command": "sh"}, Input: kemberv1.TaskInput{Ref: "s3://security-artifacts/project-a/source.tar.gz"}, TimeoutSeconds: 60}}
	if _, err := resolveTemplate(testWorkerPool(), taskRun); err == nil {
		t.Fatal("resolveTemplate() succeeded for unknown parameter")
	}
}

func testWorkerPool() *kemberv1.WorkerPool {
	return &kemberv1.WorkerPool{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID("workerpool-uid"), Generation: 3},
		Spec: kemberv1.WorkerPoolSpec{
			Execution: kemberv1.ExecutionSpec{Mode: "job"},
			Template: kemberv1.WorkerTemplate{
				Image:              "ghcr.io/example/scan-worker@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Command:            []string{"/app/scan-worker"},
				ArgsTemplate:       []string{"--severity", "{{params.severity}}", "{{input.ref}}"},
				ServiceAccountName: "scan-worker",
				InputPolicy:        kemberv1.InputPolicy{AllowedPrefixes: []string{"s3://security-artifacts/project-a/"}},
				AllowedParameters:  map[string]kemberv1.ParameterSchema{"severity": {Type: "string", Enum: []string{"HIGH", "CRITICAL"}}},
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("100m"),
				}},
			},
			TaskPolicy: kemberv1.TaskPolicySpec{QueueTimeoutSeconds: 30, TimeoutSeconds: 120, RetentionSeconds: 60},
		},
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
