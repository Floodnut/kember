package dev.kember.api.http

import dev.kember.api.model.ClusterId
import dev.kember.api.model.ResourceRef
import dev.kember.api.model.TaskRunView
import dev.kember.api.model.WorkerPoolCapacityView
import dev.kember.api.model.WorkerPoolView
import dev.kember.api.policy.SingleNamespaceAccessPolicy
import dev.kember.api.repository.KemberResourceRepository
import dev.kember.api.repository.RepositoryUnavailable
import dev.kember.api.service.KemberQueryService
import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import io.ktor.http.HttpStatusCode
import io.ktor.server.testing.testApplication
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files

class ApiModuleTest {
    @Test
    fun servesApiAndSpaFromTheSameApplication() = testApplication {
        val root = Files.createTempDirectory("kember-combined-test")
        Files.writeString(root.resolve("index.html"), "<html>Kember UI</html>")
        application {
            kemberApi(service(FakeRepository()))
            kemberUi(root)
        }

        assertEquals(HttpStatusCode.OK, client.get("/api/v1/namespaces").status)
        assertTrue(client.get("/worker-pools/example").bodyAsText().contains("Kember UI"))
        assertEquals(HttpStatusCode.NotFound, client.get("/api/v1/unknown").status)
    }

    @Test
    fun servesHealthAndVisibleNamespaces() = testApplication {
        application { kemberApi(service(FakeRepository())) }

        assertEquals(HttpStatusCode.OK, client.get("/healthz").status)
        val response = client.get("/api/v1/namespaces")
        assertEquals(HttpStatusCode.OK, response.status)
        assertEquals("""{"items":[{"cluster":"local","name":"team-a"}]}""", response.bodyAsText())
    }

    @Test
    fun servesNamespacedListsAndDetailsWithStableIdentity() = testApplication {
        application {
            kemberApi(service(FakeRepository(
                workerPools = listOf(workerPool("scanner")),
                taskRuns = listOf(taskRun("scan")),
            )))
        }

        val poolList = client.get("/api/v1/namespaces/team-a/worker-pools")
        assertEquals(HttpStatusCode.OK, poolList.status)
        assertTrue(poolList.bodyAsText().contains("\"cluster\":\"local\""))
        assertTrue(poolList.bodyAsText().contains("\"namespace\":\"team-a\""))
        assertTrue(poolList.bodyAsText().contains("\"name\":\"scanner\""))

        val taskDetail = client.get("/api/v1/namespaces/team-a/task-runs/scan")
        assertEquals(HttpStatusCode.OK, taskDetail.status)
        assertTrue(taskDetail.bodyAsText().contains("\"name\":\"scan\""))
        assertTrue(taskDetail.bodyAsText().contains("\"queueWaitSeconds\":1.123"))
        assertTrue(taskDetail.bodyAsText().contains("\"activeDurationSeconds\":2.377"))
    }

    @Test
    fun returnsEmptyItemEnvelope() = testApplication {
        application { kemberApi(service(FakeRepository())) }

        val response = client.get("/api/v1/namespaces/team-a/task-runs")

        assertEquals(HttpStatusCode.OK, response.status)
        assertEquals("""{"items":[]}""", response.bodyAsText())
    }

    @Test
    fun mapsInvalidDeniedAndMissingResources() = testApplication {
        application { kemberApi(service(FakeRepository())) }

        val invalid = client.get("/api/v1/namespaces/Team-a/worker-pools")
        assertEquals(HttpStatusCode.BadRequest, invalid.status)
        assertTrue(invalid.bodyAsText().contains("invalid_resource_identifier"))

        val denied = client.get("/api/v1/namespaces/team-b/worker-pools")
        assertEquals(HttpStatusCode.Forbidden, denied.status)
        assertTrue(denied.bodyAsText().contains("namespace_not_allowed"))
        assertFalse(denied.bodyAsText().contains("team-a"))

        val missing = client.get("/api/v1/namespaces/team-a/worker-pools/missing")
        assertEquals(HttpStatusCode.NotFound, missing.status)
        assertTrue(missing.bodyAsText().contains("resource_not_found"))
    }

    @Test
    fun hidesKubernetesFailureDetails() = testApplication {
        application { kemberApi(service(FakeRepository(fail = true))) }

        val response = client.get("/api/v1/namespaces/team-a/worker-pools")

        assertEquals(HttpStatusCode.ServiceUnavailable, response.status)
        assertTrue(response.bodyAsText().contains("kubernetes_api_unavailable"))
        assertFalse(response.bodyAsText().contains("secret backend detail"))
    }
}

private fun service(repository: KemberResourceRepository) = KemberQueryService(
    cluster = ClusterId("local"),
    accessPolicy = SingleNamespaceAccessPolicy("team-a"),
    repository = repository,
)

private class FakeRepository(
    private val workerPools: List<WorkerPoolView> = emptyList(),
    private val taskRuns: List<TaskRunView> = emptyList(),
    private val fail: Boolean = false,
) : KemberResourceRepository {
    private fun unavailable() {
        if (fail) throw RepositoryUnavailable(IllegalStateException("secret backend detail"))
    }

    override suspend fun listWorkerPools(cluster: ClusterId, namespace: String): List<WorkerPoolView> {
        unavailable()
        return workerPools
    }

    override suspend fun getWorkerPool(ref: ResourceRef): WorkerPoolView? {
        unavailable()
        return workerPools.find { it.name == ref.name }
    }

    override suspend fun listTaskRuns(cluster: ClusterId, namespace: String): List<TaskRunView> {
        unavailable()
        return taskRuns
    }

    override suspend fun getTaskRun(ref: ResourceRef): TaskRunView? {
        unavailable()
        return taskRuns.find { it.name == ref.name }
    }
}

private fun workerPool(name: String) = WorkerPoolView(
    "local", "team-a", name, 1, "exec", "warmLease",
    WorkerPoolCapacityView(1, 0, 1, 0, 0), emptyList(),
)

private fun taskRun(name: String) = TaskRunView(
    "local", "team-a", name, null, "scanner", "Pending", null, null, null, emptyList(),
    queueWaitSeconds = 1.123,
    activeDurationSeconds = 2.377,
)
