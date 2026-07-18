package dev.kember.api.model

@JvmInline
value class ClusterId(val value: String)

data class ResourceRef(
    val cluster: ClusterId,
    val namespace: String,
    val name: String,
)

data class ConditionView(
    val type: String,
    val status: String,
    val reason: String?,
    val message: String?,
    val lastTransitionTime: String?,
)

data class WorkerPoolCapacityView(
    val desired: Int,
    val starting: Int,
    val ready: Int,
    val leased: Int,
    val terminating: Int,
)

data class WorkerPoolView(
    val cluster: String,
    val namespace: String,
    val name: String,
    val generation: Long,
    val executionMode: String?,
    val lifecycleProfile: String?,
    val capacity: WorkerPoolCapacityView,
    val conditions: List<ConditionView>,
)

data class TaskRunView(
    val cluster: String,
    val namespace: String,
    val name: String,
    val createdAt: String?,
    val workerPool: String,
    val phase: String?,
    val assignedWorker: String?,
    val dispatchedAt: String?,
    val completedAt: String?,
    val conditions: List<ConditionView>,
    val queueWaitSeconds: Double? = null,
    val activeDurationSeconds: Double? = null,
)
