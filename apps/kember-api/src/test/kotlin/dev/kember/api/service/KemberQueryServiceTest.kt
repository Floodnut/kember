package dev.kember.api.service

import dev.kember.api.model.ClusterId
import dev.kember.api.model.ResourceRef
import dev.kember.api.model.TaskRunView
import dev.kember.api.model.WorkerPoolCapacityView
import dev.kember.api.model.WorkerPoolView
import dev.kember.api.policy.SingleNamespaceAccessPolicy
import dev.kember.api.repository.KemberResourceRepository
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class KemberQueryServiceTest {
    @Test
    fun exposesConfiguredNamespaceWithLocalCluster() {
        val service = service(FakeRepository())

        assertEquals(
            listOf(NamespaceView(cluster = "local", name = "team-a")),
            service.listNamespaces(),
        )
    }

    @Test
    fun rejectsNamespaceBeforeRepositoryAccess() = runBlocking {
        val repository = FakeRepository()
        val service = service(repository)

        assertThrows(NamespaceNotAllowed::class.java) {
            runBlocking { service.listWorkerPools("team-b") }
        }
        assertEquals(0, repository.calls)
    }

    @Test
    fun sortsWorkerPoolsByName() = runBlocking {
        val repository = FakeRepository(
            workerPools = listOf(workerPool("zeta"), workerPool("alpha")),
        )

        val pools = service(repository).listWorkerPools("team-a")

        assertEquals(listOf("alpha", "zeta"), pools.map { it.name })
    }

    @Test
    fun returnsExistingTaskRunAndRejectsMissingOne() = runBlocking {
        val repository = FakeRepository(taskRuns = listOf(taskRun("scan")))
        val service = service(repository)

        assertEquals("scan", service.getTaskRun("team-a", "scan").name)
        assertThrows(ResourceNotFound::class.java) {
            runBlocking { service.getTaskRun("team-a", "missing") }
        }
        Unit
    }

    private fun service(repository: FakeRepository) = KemberQueryService(
        cluster = ClusterId("local"),
        accessPolicy = SingleNamespaceAccessPolicy("team-a"),
        repository = repository,
    )
}

private class FakeRepository(
    private val workerPools: List<WorkerPoolView> = emptyList(),
    private val taskRuns: List<TaskRunView> = emptyList(),
) : KemberResourceRepository {
    var calls: Int = 0

    override suspend fun listWorkerPools(cluster: ClusterId, namespace: String): List<WorkerPoolView> {
        calls++
        return workerPools
    }

    override suspend fun getWorkerPool(ref: ResourceRef): WorkerPoolView? {
        calls++
        return workerPools.find { it.name == ref.name }
    }

    override suspend fun listTaskRuns(cluster: ClusterId, namespace: String): List<TaskRunView> {
        calls++
        return taskRuns
    }

    override suspend fun getTaskRun(ref: ResourceRef): TaskRunView? {
        calls++
        return taskRuns.find { it.name == ref.name }
    }
}

private fun workerPool(name: String) = WorkerPoolView(
    cluster = "local",
    namespace = "team-a",
    name = name,
    generation = 1,
    executionMode = "exec",
    lifecycleProfile = "warmLease",
    capacity = WorkerPoolCapacityView(1, 0, 1, 0, 0),
    conditions = emptyList(),
)

private fun taskRun(name: String) = TaskRunView(
    cluster = "local",
    namespace = "team-a",
    name = name,
    createdAt = null,
    workerPool = "scanner",
    phase = "Pending",
    assignedWorker = null,
    dispatchedAt = null,
    completedAt = null,
    conditions = emptyList(),
)
