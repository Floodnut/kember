// Package v1alpha1 defines Kember's Kubernetes API.
// +kubebuilder:object:generate=true
// +groupName=kember.dev
package v1alpha1

import (
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "kember.dev", Version: "v1alpha1"}

// AddToScheme registers Kember types with a scheme.
func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &WorkerPool{}, &WorkerPoolList{}, &TaskRun{}, &TaskRunList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

type WorkerPoolSpec struct {
	Execution  ExecutionSpec  `json:"execution"`
	Lifecycle  *LifecycleSpec `json:"lifecycle,omitempty"`
	Capacity   *CapacitySpec  `json:"capacity,omitempty"`
	Template   WorkerTemplate `json:"template"`
	TaskPolicy TaskPolicySpec `json:"taskPolicy"`
}

type ExecutionSpec struct {
	// +kubebuilder:validation:Enum=job;exec
	Mode            string   `json:"mode"`
	CommandTemplate []string `json:"commandTemplate,omitempty"`
}

type LifecycleSpec struct {
	// +kubebuilder:validation:Enum=runToCompletion;warmLease
	Profile string `json:"profile"`
	// +kubebuilder:validation:Minimum=1
	MaxTasksPerWorker int32 `json:"maxTasksPerWorker"`
}

type CapacitySpec struct {
	// +kubebuilder:validation:Enum=fixed
	Policy string `json:"policy"`
	// +kubebuilder:validation:Minimum=1
	Size int32 `json:"size"`
}

type WorkerTemplate struct {
	Image              string                      `json:"image"`
	Command            []string                    `json:"command"`
	Args               []string                    `json:"args,omitempty"`
	ArgsTemplate       []string                    `json:"argsTemplate,omitempty"`
	ServiceAccountName string                      `json:"serviceAccountName"`
	InputPolicy        InputPolicy                 `json:"inputPolicy"`
	AllowedParameters  map[string]ParameterSchema  `json:"allowedParameters,omitempty"`
	Resources          corev1.ResourceRequirements `json:"resources"`
	SecurityContext    *corev1.SecurityContext     `json:"securityContext,omitempty"`
	ReadinessProbe     *corev1.Probe               `json:"readinessProbe,omitempty"`
}

type InputPolicy struct {
	AllowedPrefixes []string `json:"allowedPrefixes"`
}

type ParameterSchema struct {
	// +kubebuilder:validation:Enum=string
	Type string   `json:"type"`
	Enum []string `json:"enum,omitempty"`
}

type TaskPolicySpec struct {
	// +kubebuilder:validation:Minimum=1
	QueueTimeoutSeconds int64 `json:"queueTimeoutSeconds"`
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds int64 `json:"timeoutSeconds"`
	// +kubebuilder:validation:Minimum=0
	RetentionSeconds int32 `json:"retentionSeconds"`
}

// WorkerPool is a platform-owned execution template.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=wp
type WorkerPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkerPoolSpec `json:"spec"`
}

// +kubebuilder:object:root=true
type WorkerPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkerPool `json:"items"`
}

type TaskRunSpec struct {
	WorkerPoolRef WorkerPoolReference `json:"workerPoolRef"`
	Parameters    map[string]string   `json:"parameters,omitempty"`
	Input         TaskInput           `json:"input"`
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds int64 `json:"timeoutSeconds"`
	Cancel         bool  `json:"cancel,omitempty"`
}

type WorkerPoolReference struct {
	Name string `json:"name"`
}

type TaskInput struct {
	Ref string `json:"ref"`
}

type TaskRunPhase string

const (
	TaskRunPending   TaskRunPhase = "Pending"
	TaskRunRunning   TaskRunPhase = "Running"
	TaskRunSucceeded TaskRunPhase = "Succeeded"
	TaskRunFailed    TaskRunPhase = "Failed"
	TaskRunTimedOut  TaskRunPhase = "TimedOut"
	TaskRunRejected  TaskRunPhase = "Rejected"
	TaskRunCancelled TaskRunPhase = "Cancelled"
)

func (phase TaskRunPhase) IsTerminal() bool {
	return phase == TaskRunSucceeded || phase == TaskRunFailed || phase == TaskRunTimedOut || phase == TaskRunRejected || phase == TaskRunCancelled
}

type ResolvedTemplate struct {
	WorkerPoolUID           string                      `json:"workerPoolUID"`
	WorkerPoolGeneration    int64                       `json:"workerPoolGeneration"`
	Image                   string                      `json:"image"`
	Command                 []string                    `json:"command"`
	Args                    []string                    `json:"args"`
	ExecutionMode           string                      `json:"executionMode"`
	LifecycleProfile        string                      `json:"lifecycleProfile"`
	ExecCommand             []string                    `json:"execCommand,omitempty"`
	ServiceAccountName      string                      `json:"serviceAccountName"`
	Resources               corev1.ResourceRequirements `json:"resources"`
	SecurityContext         *corev1.SecurityContext     `json:"securityContext,omitempty"`
	ActiveDeadlineSeconds   int64                       `json:"activeDeadlineSeconds"`
	QueueTimeoutSeconds     int64                       `json:"queueTimeoutSeconds"`
	TTLSecondsAfterFinished int32                       `json:"ttlSecondsAfterFinished"`
	TemplateHash            string                      `json:"templateHash"`
}

type JobReference struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type WorkerReference struct {
	Name      string `json:"name"`
	UID       string `json:"uid"`
	LeaseName string `json:"leaseName"`
}

type TaskRunStatus struct {
	Phase            TaskRunPhase       `json:"phase,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
	ResolvedTemplate *ResolvedTemplate  `json:"resolvedTemplate,omitempty"`
	JobRef           *JobReference      `json:"jobRef,omitempty"`
	WorkerRef        *WorkerReference   `json:"workerRef,omitempty"`
	StartedAt        *metav1.Time       `json:"startedAt,omitempty"`
	CompletedAt      *metav1.Time       `json:"completedAt,omitempty"`
	ExitCode         *int32             `json:"exitCode,omitempty"`
	Result           *TaskResult        `json:"result,omitempty"`
}

type TaskResult struct {
	Summary string `json:"summary"`
}

// TaskRun is a tenant-owned request to execute a WorkerPool template once.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=tr
type TaskRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TaskRunSpec   `json:"spec"`
	Status            TaskRunStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TaskRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TaskRun `json:"items"`
}

func (in *WorkerPool) DeepCopyObject() runtime.Object     { return jsonDeepCopy(in, &WorkerPool{}) }
func (in *WorkerPoolList) DeepCopyObject() runtime.Object { return jsonDeepCopy(in, &WorkerPoolList{}) }
func (in *TaskRun) DeepCopyObject() runtime.Object        { return jsonDeepCopy(in, &TaskRun{}) }
func (in *TaskRunList) DeepCopyObject() runtime.Object    { return jsonDeepCopy(in, &TaskRunList{}) }

func jsonDeepCopy[T runtime.Object](in T, out T) T {
	if any(in) == nil {
		return out
	}
	data, _ := json.Marshal(in)
	_ = json.Unmarshal(data, out)
	return out
}
