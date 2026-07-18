package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
)

// LifecycleMetrics exposes bounded-cardinality operational lifecycle signals.
type LifecycleMetrics struct {
	workerPoolReady   *prometheus.GaugeVec
	workerPoolLeased  *prometheus.GaugeVec
	taskRunActive     *prometheus.HistogramVec
	taskRunTotal      *prometheus.CounterVec
	workerTermination *prometheus.CounterVec
	assignmentWait    *prometheus.HistogramVec
}

func NewLifecycleMetrics(registerer prometheus.Registerer) *LifecycleMetrics {
	buckets := prometheus.ExponentialBuckets(0.05, 2, 16)
	metrics := &LifecycleMetrics{
		workerPoolReady: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "kember_workerpool_ready_workers",
			Help: "Current number of Ready, unleased workers in a WorkerPool.",
		}, []string{"namespace", "workerpool"}),
		workerPoolLeased: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "kember_workerpool_leased_workers",
			Help: "Current number of leased workers in a WorkerPool.",
		}, []string{"namespace", "workerpool"}),
		taskRunActive: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "kember_taskrun_active_duration_seconds",
			Help:    "TaskRun duration from dispatch until terminal status.",
			Buckets: buckets,
		}, []string{"execution_mode", "lifecycle_profile"}),
		taskRunTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "kember_taskrun_total",
			Help: "TaskRuns persisted in a terminal phase by phase and reason.",
		}, []string{"phase", "reason"}),
		workerTermination: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "kember_worker_termination_requests_total",
			Help: "Worker Pod deletion requests accepted by the API server by reason.",
		}, []string{"reason"}),
		assignmentWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "kember_taskrun_assignment_wait_seconds",
			Help:    "TaskRun duration from creation until dispatch.",
			Buckets: buckets,
		}, []string{"execution_mode", "lifecycle_profile"}),
	}
	registerer.MustRegister(metrics.workerPoolReady, metrics.workerPoolLeased, metrics.taskRunActive, metrics.taskRunTotal, metrics.workerTermination, metrics.assignmentWait)
	return metrics
}

func (m *LifecycleMetrics) SetWorkerPool(namespace, name string, ready, leased int32) {
	if m == nil {
		return
	}
	m.workerPoolReady.WithLabelValues(namespace, name).Set(float64(ready))
	m.workerPoolLeased.WithLabelValues(namespace, name).Set(float64(leased))
}

func (m *LifecycleMetrics) DeleteWorkerPool(namespace, name string) {
	if m == nil {
		return
	}
	m.workerPoolReady.DeleteLabelValues(namespace, name)
	m.workerPoolLeased.DeleteLabelValues(namespace, name)
}

func (m *LifecycleMetrics) ObserveTaskRunTerminal(taskRun *kemberv1.TaskRun, reason string) {
	if m == nil {
		return
	}
	m.taskRunTotal.WithLabelValues(string(taskRun.Status.Phase), reason).Inc()
	if taskRun.Status.ResolvedTemplate == nil || taskRun.Status.DispatchedAt == nil || taskRun.Status.CompletedAt == nil {
		return
	}
	labels := []string{taskRun.Status.ResolvedTemplate.ExecutionMode, taskRun.Status.ResolvedTemplate.LifecycleProfile}
	observeNonNegative(m.assignmentWait.WithLabelValues(labels...), taskRun.Status.DispatchedAt.Sub(taskRun.CreationTimestamp.Time))
	observeNonNegative(m.taskRunActive.WithLabelValues(labels...), taskRun.Status.CompletedAt.Sub(taskRun.Status.DispatchedAt.Time))
}

func (m *LifecycleMetrics) ObserveWorkerTermination(reason string) {
	if m != nil {
		m.workerTermination.WithLabelValues(reason).Inc()
	}
}

func observeNonNegative(observer prometheus.Observer, duration time.Duration) {
	if duration >= 0 {
		observer.Observe(duration.Seconds())
	}
}
