# Worker Contract

이 패키지는 `WorkerPool`, `TaskRun`, `WorkerSession`의 언어 중립 계약을 보관한다.

CRD Go type, Kotlin API DTO, TypeScript client type은 이 계약에서 파생한다. 초기 구현에서는 CRD 스키마가 확정된 뒤 protobuf 또는 OpenAPI 기반 생성 방식을 선택한다.
