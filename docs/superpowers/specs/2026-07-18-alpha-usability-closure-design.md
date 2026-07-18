# Kember Alpha Usability Closure 설계

## 핵심 목표

현재 구현된 Kember Operator를 새로운 기능 없이 설치·실행·관찰할 수 있는
alpha 수준으로 닫는다. 이 작업의 성공 기준은 기능 수가 아니라, 깨끗한
Kubernetes 클러스터에서 사용자가 WorkerPool과 TaskRun의 lifecycle을 재현하고
해석할 수 있는지다.

## 요약

이번 단계는 `Alpha Usability Closure`라는 하나의 작업 묶음으로 진행한다.

- CRD schema와 controller 동작의 불일치를 찾는다.
- clean kind cluster 설치와 sample 실행을 one-shot smoke로 검증한다.
- 핵심 terminal phase와 실패 경로만 자동 검증한다.
- 설치, 제출, 취소, status, metrics 사용법을 README/deploy 문서와 일치시킨다.
- Kotlin API, UI, plugin, Helm, multi-cluster, reusable worker는 범위에서 제외한다.

## 배경 및 상황

Kember는 WarmLease와 Job execution path, lifecycle metrics, CRD status를 이미
구현했고 Checkov·Trivy benchmark로 핵심 가치도 검증했다. 현재 남은 위험은 새
기능 부족이 아니라 다음 세 가지다.

1. 설치 manifest와 실제 operator 동작이 clean cluster에서 함께 검증되지 않았다.
2. CRD가 허용하는 상태와 controller가 기록하는 상태의 회귀를 자동으로 잡는
   최소 계약 테스트가 부족하다.
3. 사용자가 어떤 phase와 metric을 관찰해야 하는지 한 경로로 설명되지 않았다.

따라서 이 단계는 performance 재실증이 아니라 설치·계약·관찰성의 closure다.

## 대안 및 트레이드오프

### A. 대규모 E2E 확장

모든 acceptance scenario를 별도 kind test로 만든다. 회귀 방지는 강하지만,
작은 프로젝트의 핵심보다 test harness 유지 비용이 커진다.

### B. 문서만 정리

설치와 API 문서를 먼저 완성하고 수동으로 확인한다. 빠르지만 controller와
문서가 다시 어긋날 위험이 있다.

### C. 핵심 자동화와 문서 동기화 (채택)

설치·sample lifecycle·terminal phase·metrics family만 짧은 smoke/unit test로
자동화하고, 전체 운영 시나리오는 기존 E2E와 문서로 연결한다. 구현량과 회귀
위험의 균형이 가장 좋다.

## 설계

### 1. 설치 smoke

`deploy/install.sh`를 clean kind cluster에서 실행한다.

검증 순서:

1. namespace, CRD, RBAC, Deployment 적용
2. operator Deployment rollout 완료
3. Job WorkerPool과 WarmLease WorkerPool 적용
4. 각각의 TaskRun 생성
5. `Succeeded`와 replacement capacity 관찰
6. operator metrics endpoint에서 lifecycle metric family 확인

설치 스크립트는 현재 kubectl context만 사용하고 tenant namespace나 sample
resource를 자동 생성하지 않는다. operator image는 실행 전에 cluster에
존재해야 한다.

### 2. 계약 테스트

다음 계약만 alpha의 필수 자동 검증으로 둔다.

| 계약 | 기대 결과 |
| --- | --- |
| digest 없는 image | CRD admission 거부 |
| URI 형식이 아닌 input.ref | CRD admission 거부 |
| 허용 prefix 밖 input.ref | TaskRun `Rejected` |
| 허용되지 않은 parameter | TaskRun `Rejected` |
| WorkerPool 정책보다 긴 timeout | TaskRun `Rejected` |
| cancel 요청 | Job/worker 정리 후 `Cancelled` |
| 실행 timeout | Job/worker 정리 후 `TimedOut` |
| command exit 0 | `Succeeded` |
| command non-zero | `Failed` |
| terminal TaskRun 재조정 | phase 불변, 재실행 없음 |

정적 WorkerPool 오류는 CRD admission, WorkerPool 참조·parameter·input·timeout
오류는 controller 동적 validation으로 구분한다.

### 3. 상태와 metrics 관찰

TaskRun phase는 `Pending` 또는 `Running`에서 하나의 terminal phase로 단조
전이한다. terminal phase는 `Succeeded`, `Failed`, `TimedOut`, `Rejected`,
`Cancelled`다. 사용자는 `status.conditions`, `resolvedTemplate`, `jobRef` 또는
`workerRef`, `dispatchedAt`, `completedAt`을 통해 lifecycle을 해석한다.

최소 metrics 사용법은 다음 family를 기준으로 한다.

- `kember_workerpool_ready_workers`
- `kember_workerpool_assigned_workers`
- `kember_taskrun_duration_seconds`
- `kember_taskrun_total`
- `kember_worker_termination_total`
- `kember_assignment_wait_seconds`

metrics는 진단·관찰용이며 CRD status를 대체하지 않는다.

### 4. 문서 동기화

README와 `deploy/README.md`는 다음 명령을 실제로 설명해야 한다.

- operator image 준비
- `deploy/install.sh` 실행
- sample WorkerPool/TaskRun 적용
- status 조회
- cancel patch
- metrics endpoint 확인

공개 저장소에는 README만 유지하고, 이 설계 문서는 로컬 검토 산출물로 둔다.

## 오류 및 운영 경계

- 설치 중 CRD/RBAC/Deployment 오류는 즉시 실패하고 후속 resource를 추정하지
  않는다.
- operator restart와 reconcile 재시도는 기존 Job/worker를 재발견해야 하며
  duplicate execution을 만들지 않는다.
- capacity 부족은 TaskRun을 즉시 실패시키지 않고 queue timeout까지 `Pending`으로
  유지한다.
- metrics scrape 실패가 TaskRun lifecycle 결과를 바꾸지 않는다.
- 이 단계에서는 자동 rollback, Helm upgrade 전략, multi-cluster recovery를
  구현하지 않는다.

## 테스트 전략

- Go unit: validation, phase terminal 불변성, status snapshot
- shell syntax: install 및 기존 E2E/benchmark script
- YAML parse: CRD와 sample manifest
- kind smoke: 설치부터 Job/WarmLease 성공과 metrics 조회까지
- 기존 benchmark: 성능 결론을 다시 만들지 않고 회귀 여부만 확인

Bazel 테스트는 실행 환경에 Bazel이 있을 때 필수로 실행한다. kind smoke를
실행할 수 없는 환경에서는 Go·shell·YAML 검증을 먼저 수행하고, alpha 종료
조건은 충족된 것으로 표시하지 않는다.

## 결론

Alpha Usability Closure는 Kember의 범위를 넓히지 않는다. WarmLease의 가치와
현재 lifecycle 구현을 실제 사용 가능한 설치·계약·관찰 경로로 묶는 마지막
정리 단계다. 이 단계가 통과한 뒤에만 plugin SPI나 Kotlin API/UI를 별도 설계
대상으로 승격한다.

## 액션 아이템

1. clean kind에서 `deploy/install.sh` smoke 실행
2. 계약 표의 누락 unit/E2E 테스트 추가
3. sample 적용과 status/metrics 조회 명령 검증
4. README와 deploy 문서의 명령·출력 동기화
5. 테스트 결과와 미지원 환경을 alpha checklist에 기록

## 참고

- `README.md`
- `deploy/README.md`
- `deploy/install.sh`
- `deploy/crd/kember.dev_workerpools.yaml`
- `deploy/crd/kember.dev_taskruns.yaml`
- `apps/kember-operator/controller/taskrun.go`
- `apps/kember-operator/controller/workerpool.go`
