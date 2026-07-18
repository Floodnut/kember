package dev.kember.api.repository

import dev.kember.api.model.ClusterId
import dev.kember.api.model.ConditionView
import dev.kember.api.model.ResourceRef
import dev.kember.api.model.TaskRunView
import dev.kember.api.model.WorkerPoolCapacityView
import dev.kember.api.model.WorkerPoolView
import io.fabric8.kubernetes.api.model.GenericKubernetesResource
import io.fabric8.kubernetes.client.KubernetesClient
import io.fabric8.kubernetes.client.KubernetesClientException
import io.fabric8.kubernetes.client.dsl.base.ResourceDefinitionContext
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.withContext

class Fabric8KemberResourceRepository(
    private val client: KubernetesClient,
    private val ioDispatcher: CoroutineDispatcher,
) : KemberResourceRepository {
    override suspend fun listWorkerPools(cluster: ClusterId, namespace: String): List<WorkerPoolView> =
        kubernetesCall {
            client.genericKubernetesResources(WORKER_POOLS)
                .inNamespace(namespace)
                .list()
                .items
                .map { it.toWorkerPool(cluster, namespace) }
        }

    override suspend fun getWorkerPool(ref: ResourceRef): WorkerPoolView? =
        kubernetesCall {
            client.genericKubernetesResources(WORKER_POOLS)
                .inNamespace(ref.namespace)
                .withName(ref.name)
                .get()
                ?.toWorkerPool(ref.cluster, ref.namespace)
        }

    override suspend fun listTaskRuns(cluster: ClusterId, namespace: String): List<TaskRunView> =
        kubernetesCall {
            client.genericKubernetesResources(TASK_RUNS)
                .inNamespace(namespace)
                .list()
                .items
                .map { it.toTaskRun(cluster, namespace) }
        }

    override suspend fun getTaskRun(ref: ResourceRef): TaskRunView? =
        kubernetesCall {
            client.genericKubernetesResources(TASK_RUNS)
                .inNamespace(ref.namespace)
                .withName(ref.name)
                .get()
                ?.toTaskRun(ref.cluster, ref.namespace)
        }

    private suspend fun <T> kubernetesCall(block: () -> T): T = withContext(ioDispatcher) {
        try {
            block()
        } catch (error: KubernetesClientException) {
            throw RepositoryUnavailable(error)
        }
    }
}

private fun GenericKubernetesResource.toWorkerPool(cluster: ClusterId, namespace: String) = WorkerPoolView(
    cluster = cluster.value,
    namespace = metadata.namespace ?: namespace,
    name = metadata.name,
    generation = metadata.generation ?: 0,
    executionMode = string("spec", "execution", "mode"),
    lifecycleProfile = string("spec", "lifecycle", "profile"),
    capacity = WorkerPoolCapacityView(
        desired = int("status", "capacity", "desired"),
        starting = int("status", "capacity", "starting"),
        ready = int("status", "capacity", "ready"),
        leased = int("status", "capacity", "leased"),
        terminating = int("status", "capacity", "terminating"),
    ),
    conditions = conditions(),
)

private fun GenericKubernetesResource.toTaskRun(cluster: ClusterId, namespace: String): TaskRunView {
    val createdAt = metadata.creationTimestamp
    val dispatchedAt = string("status", "dispatchedAt")
    val completedAt = string("status", "completedAt")
    return TaskRunView(
        cluster = cluster.value,
        namespace = metadata.namespace ?: namespace,
        name = metadata.name,
        createdAt = createdAt,
        workerPool = string("spec", "workerPoolRef", "name") ?: "",
        phase = string("status", "phase"),
        assignedWorker = string("status", "workerRef", "name"),
        dispatchedAt = dispatchedAt,
        completedAt = completedAt,
        conditions = conditions(),
        queueWaitSeconds = durationSeconds(createdAt, dispatchedAt),
        activeDurationSeconds = durationSeconds(dispatchedAt, completedAt),
    )
}

private fun durationSeconds(start: String?, end: String?): Double? {
    if (start == null || end == null) return null
    val duration = runCatching {
        java.time.Duration.between(java.time.Instant.parse(start), java.time.Instant.parse(end))
    }.getOrNull() ?: return null
    if (duration.isNegative) return null
    return duration.toNanos().toDouble() / 1_000_000_000.0
}

private fun GenericKubernetesResource.string(vararg path: Any): String? =
    get<Any>(*path)?.toString()

private fun GenericKubernetesResource.int(vararg path: Any): Int =
    (get<Any>(*path) as? Number)?.toInt() ?: 0

private fun GenericKubernetesResource.conditions(): List<ConditionView> {
    val values = get<Any>("status", "conditions") as? Collection<*> ?: return emptyList()
    return values.mapNotNull { raw ->
        val condition = raw as? Map<*, *> ?: return@mapNotNull null
        ConditionView(
            type = condition["type"]?.toString() ?: return@mapNotNull null,
            status = condition["status"]?.toString() ?: "Unknown",
            reason = condition["reason"]?.toString(),
            message = condition["message"]?.toString(),
            lastTransitionTime = condition["lastTransitionTime"]?.toString(),
        )
    }
}

private fun context(kind: String, plural: String) = ResourceDefinitionContext.Builder()
    .withGroup("kember.openflood.org")
    .withVersion("v1alpha1")
    .withKind(kind)
    .withPlural(plural)
    .withNamespaced(true)
    .build()

private val WORKER_POOLS = context("WorkerPool", "workerpools")
private val TASK_RUNS = context("TaskRun", "taskruns")
