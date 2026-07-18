package controller

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

func TestLifecycleMetricsExposeCapacityAndRemoveDeletedPool(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewLifecycleMetrics(registry)
	metrics.SetWorkerPool("security", "scanner", 2, 1)

	ready, ok := gatherMetric(registry, "kember_workerpool_ready_workers", map[string]string{"namespace": "security", "workerpool": "scanner"})
	if !ok || ready.value != 2 {
		t.Fatalf("ready worker gauge = %+v, %t, want 2", ready, ok)
	}
	leased, ok := gatherMetric(registry, "kember_workerpool_leased_workers", map[string]string{"namespace": "security", "workerpool": "scanner"})
	if !ok || leased.value != 1 {
		t.Fatalf("leased worker gauge = %+v, %t, want 1", leased, ok)
	}

	metrics.DeleteWorkerPool("security", "scanner")
	if _, ok := gatherMetric(registry, "kember_workerpool_ready_workers", map[string]string{"namespace": "security", "workerpool": "scanner"}); ok {
		t.Fatal("deleted WorkerPool gauge series still exists")
	}
}

func TestLifecycleMetricsObserveTerminalTaskRunIntervals(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewLifecycleMetrics(registry)
	created := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	dispatched := kemberv1.NewPreciseTime(created.Add(2 * time.Second))
	completed := kemberv1.NewPreciseTime(created.Add(5 * time.Second))
	taskRun := &kemberv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(created)},
		Status: kemberv1.TaskRunStatus{
			Phase:            kemberv1.TaskRunSucceeded,
			DispatchedAt:     &dispatched,
			CompletedAt:      &completed,
			ResolvedTemplate: &kemberv1.ResolvedTemplate{ExecutionMode: "exec", LifecycleProfile: "warmLease"},
		},
	}
	metrics.ObserveTaskRunTerminal(taskRun, "WorkerCommandSucceeded")

	total, ok := gatherMetric(registry, "kember_taskrun_total", map[string]string{"phase": "Succeeded", "reason": "WorkerCommandSucceeded"})
	if !ok || total.value != 1 {
		t.Fatalf("terminal counter = %+v, %t, want 1", total, ok)
	}
	wait, ok := gatherMetric(registry, "kember_taskrun_assignment_wait_seconds", map[string]string{"execution_mode": "exec", "lifecycle_profile": "warmLease"})
	if !ok || wait.count != 1 || wait.sum != 2 {
		t.Fatalf("assignment histogram = %+v, %t, want count=1 sum=2", wait, ok)
	}
	active, ok := gatherMetric(registry, "kember_taskrun_active_duration_seconds", map[string]string{"execution_mode": "exec", "lifecycle_profile": "warmLease"})
	if !ok || active.count != 1 || active.sum != 3 {
		t.Fatalf("active histogram = %+v, %t, want count=1 sum=3", active, ok)
	}
}

func TestTaskRunTimingPreservesSubsecondPrecision(t *testing.T) {
	start := time.Date(2026, 7, 18, 1, 0, 0, 100_000_000, time.UTC)
	dispatched := kemberv1.NewPreciseTime(start)
	completed := kemberv1.NewPreciseTime(start.Add(175 * time.Millisecond))
	original := &kemberv1.TaskRun{Status: kemberv1.TaskRunStatus{DispatchedAt: &dispatched, CompletedAt: &completed}}
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &kemberv1.TaskRun{}
	if err := json.Unmarshal(payload, decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Status.CompletedAt.Sub(decoded.Status.DispatchedAt.Time); got != 175*time.Millisecond {
		t.Fatalf("serialized active duration = %s, want 175ms", got)
	}
}

func TestTaskRunTimingReadsLegacySecondPrecision(t *testing.T) {
	payload := []byte(`{"status":{"dispatchedAt":"2026-07-18T01:00:00Z","completedAt":"2026-07-18T01:00:03Z"}}`)
	decoded := &kemberv1.TaskRun{}
	if err := json.Unmarshal(payload, decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Status.CompletedAt.Sub(decoded.Status.DispatchedAt.Time); got != 3*time.Second {
		t.Fatalf("legacy active duration = %s, want 3s", got)
	}
}

func TestLifecycleMetricsSkipIntervalsBeforeDispatch(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewLifecycleMetrics(registry)
	metrics.ObserveTaskRunTerminal(&kemberv1.TaskRun{Status: kemberv1.TaskRunStatus{Phase: kemberv1.TaskRunRejected}}, "TaskRejected")

	if _, ok := gatherMetric(registry, "kember_taskrun_assignment_wait_seconds", nil); ok {
		t.Fatal("undispatched TaskRun produced assignment histogram")
	}
	if total, ok := gatherMetric(registry, "kember_taskrun_total", map[string]string{"phase": "Rejected", "reason": "TaskRejected"}); !ok || total.value != 1 {
		t.Fatalf("rejected counter = %+v, %t, want 1", total, ok)
	}
}

type metricSnapshot struct {
	value float64
	count uint64
	sum   float64
}

func gatherMetric(registry *prometheus.Registry, name string, expectedLabels map[string]string) (metricSnapshot, bool) {
	families, err := registry.Gather()
	if err != nil {
		return metricSnapshot{}, false
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			labels := make(map[string]string, len(metric.Label))
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if !sameLabels(labels, expectedLabels) {
				continue
			}
			snapshot := metricSnapshot{}
			if metric.Gauge != nil {
				snapshot.value = metric.GetGauge().GetValue()
			}
			if metric.Counter != nil {
				snapshot.value = metric.GetCounter().GetValue()
			}
			if metric.Histogram != nil {
				snapshot.count = metric.GetHistogram().GetSampleCount()
				snapshot.sum = metric.GetHistogram().GetSampleSum()
			}
			return snapshot, true
		}
	}
	return metricSnapshot{}, false
}

func sameLabels(actual, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}
