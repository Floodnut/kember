# Worker Contract

이 패키지는 `WorkerPool`, `TaskRun`, `WorkerSession`과 plugin SPI의 언어 중립 계약을 보관한다.

CRD Go type, Kotlin API DTO, TypeScript client type은 이 계약에서 파생한다.

## Plugin SPI v1

[`proto/kember/plugin/v1/plugin.proto`](proto/kember/plugin/v1/plugin.proto)는
Kember core가 외부 policy adapter에 전달하는 최소 control-plane snapshot을
정의한다. 첫 capability는 `PolicyDecision`이며 결과는 `ALLOW`, `DENY`,
`ABSTAIN` 중 하나다. `ABSTAIN`은 허용을 뜻하지 않고 Kember core policy에
결정을 위임한다.

호출 측의 기본 deadline은 500ms이며 timeout은 transport error로 처리한다.
transport error가 발생하면 binding의
`failurePolicy=deny`는 TaskRun을 거부하고, `failurePolicy=allow`는 audit event를
남긴 뒤 core flow를 계속한다. binding과 실제 gRPC 호출 경로는 아직
v1alpha1 Operator에 연결하지 않았다.

Plugin에는 Kubernetes token, secret 값, input bulk data를 전달하지 않는다.
Plugin은 Job, Pod, Lease, worker capacity 또는 TaskRun lifecycle을 직접
소유하지 않는다. [`testdata`](testdata)의 fixture는 decision과 transport failure
의 기대 결과를 고정한다.

Go와 JVM protobuf type은 Bazel build 시 같은 schema에서 생성하며 generated
source는 Git에 체크인하지 않는다. Kotlin consumer는 generated JVM/Java type을
직접 사용한다. Go와 Kotlin compatibility test는 같은 `PolicyResponse`가 동일한
protobuf wire encoding을 만드는지 검증한다. 이 codegen target은 gRPC stub이나
Operator runtime integration을 포함하지 않는다.
