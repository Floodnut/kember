package dev.kember.api.service

import dev.kember.api.model.ClusterId
import dev.kember.api.model.ResourceRef
import dev.kember.api.model.TaskRunView
import dev.kember.api.model.WorkerPoolView
import dev.kember.api.policy.NamespaceAccessPolicy
import dev.kember.api.repository.KemberResourceRepository

data class NamespaceView(val cluster: String, val name: String)

class NamespaceNotAllowed : RuntimeException()

class ResourceNotFound : RuntimeException()

class KemberQueryService(
    private val cluster: ClusterId,
    private val accessPolicy: NamespaceAccessPolicy,
    private val repository: KemberResourceRepository,
) {
    fun listNamespaces(): List<NamespaceView> =
        accessPolicy.visibleNamespaces().sorted().map { NamespaceView(cluster.value, it) }

    suspend fun listWorkerPools(namespace: String): List<WorkerPoolView> {
        requireAllowed(namespace)
        return repository.listWorkerPools(cluster, namespace).sortedBy { it.name }
    }

    suspend fun getWorkerPool(namespace: String, name: String): WorkerPoolView {
        requireAllowed(namespace)
        return repository.getWorkerPool(ResourceRef(cluster, namespace, name)) ?: throw ResourceNotFound()
    }

    suspend fun listTaskRuns(namespace: String): List<TaskRunView> {
        requireAllowed(namespace)
        return repository.listTaskRuns(cluster, namespace).sortedBy { it.name }
    }

    suspend fun getTaskRun(namespace: String, name: String): TaskRunView {
        requireAllowed(namespace)
        return repository.getTaskRun(ResourceRef(cluster, namespace, name)) ?: throw ResourceNotFound()
    }

    private fun requireAllowed(namespace: String) {
        if (!accessPolicy.allows(namespace)) {
            throw NamespaceNotAllowed()
        }
    }
}
