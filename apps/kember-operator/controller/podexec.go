package controller

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	clientexec "k8s.io/client-go/util/exec"
)

// KubernetesPodExecutor implements the WarmLease exec transport through the Pod exec subresource.
type KubernetesPodExecutor struct {
	config *rest.Config
	core   typedcorev1.CoreV1Interface
}

func NewKubernetesPodExecutor(config *rest.Config, core typedcorev1.CoreV1Interface) *KubernetesPodExecutor {
	return &KubernetesPodExecutor{config: rest.CopyConfig(config), core: core}
}

func (e *KubernetesPodExecutor) Execute(ctx context.Context, namespace, pod, container string, command []string) (int32, error) {
	request := e.core.RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{Container: container, Command: command, Stdout: true, Stderr: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(e.config, "POST", request.URL())
	if err != nil {
		return -1, err
	}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: io.Discard, Stderr: io.Discard})
	if exitError, ok := err.(clientexec.ExitError); ok {
		return int32(exitError.ExitStatus()), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}
