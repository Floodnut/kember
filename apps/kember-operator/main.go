// kember-operator reconciles Kember worker lifecycle resources.
package main

import (
	"flag"
	"fmt"
	"os"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kemberv1 "github.com/Floodnut/kember/apps/kember-operator/api/v1alpha1"
	"github.com/Floodnut/kember/apps/kember-operator/controller"
)

func main() {
	var metricsAddress string
	var taskRunControllerWorkers int
	var warmExecMaxConcurrency int
	flag.StringVar(&metricsAddress, "metrics-bind-address", ":8080", "metrics endpoint address")
	flag.IntVar(&taskRunControllerWorkers, "taskrun-controller-workers", 8, "maximum concurrent TaskRun reconciles")
	flag.IntVar(&warmExecMaxConcurrency, "warm-exec-max-concurrency", 4, "maximum concurrent blocking WarmLease exec calls")
	flag.Parse()
	must(validateConcurrency(taskRunControllerWorkers, warmExecMaxConcurrency))
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	scheme := runtime.NewScheme()
	must(clientgoscheme.AddToScheme(scheme))
	must(kemberv1.AddToScheme(scheme))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme, Metrics: metricsserver.Options{BindAddress: metricsAddress}})
	must(err)
	lifecycleMetrics := controller.NewLifecycleMetrics(crmetrics.Registry)
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	must(err)
	must(ctrl.NewControllerManagedBy(mgr).For(&kemberv1.TaskRun{}).Owns(&batchv1.Job{}).WithOptions(controllerconfig.Options{MaxConcurrentReconciles: taskRunControllerWorkers}).Complete(&controller.TaskRunReconciler{Client: mgr.GetClient(), APIReader: mgr.GetAPIReader(), Scheme: mgr.GetScheme(), Executor: controller.NewKubernetesPodExecutor(mgr.GetConfig(), clientset.CoreV1()), WarmExecSlots: make(chan struct{}, warmExecMaxConcurrency), Metrics: lifecycleMetrics}))
	must(ctrl.NewControllerManagedBy(mgr).For(&kemberv1.WorkerPool{}).Owns(&corev1.Pod{}).Complete(&controller.WorkerPoolReconciler{Client: mgr.GetClient(), APIReader: mgr.GetAPIReader(), Scheme: mgr.GetScheme(), Metrics: lifecycleMetrics}))
	must(mgr.Start(ctrl.SetupSignalHandler()))
}

func validateConcurrency(taskRunControllerWorkers, warmExecMaxConcurrency int) error {
	if warmExecMaxConcurrency < 1 {
		return fmt.Errorf("warm-exec-max-concurrency must be at least 1")
	}
	if taskRunControllerWorkers <= warmExecMaxConcurrency {
		return fmt.Errorf("taskrun-controller-workers must be greater than warm-exec-max-concurrency")
	}
	return nil
}

func must(err error) {
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
