package dev.kember.api.repository

import dev.kember.api.model.ClusterId
import dev.kember.api.model.ResourceRef
import io.fabric8.kubernetes.client.server.mock.KubernetesMockServer
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

class Fabric8KemberResourceRepositoryTest {
    private lateinit var server: KubernetesMockServer

    @Before
    fun startServer() {
        server = KubernetesMockServer(false)
        server.start()
    }

    @After
    fun stopServer() {
        server.shutdown()
    }

    @Test
    fun mapsWorkerPoolProjection() = runBlocking {
        server.expect().get()
            .withPath("/apis/kember.openflood.org/v1alpha1/namespaces/team-a/workerpools")
            .andReturn(200, WORKER_POOL_LIST)
            .once()
        val client = server.createClient()

        val pools = Fabric8KemberResourceRepository(client, Dispatchers.IO)
            .listWorkerPools(ClusterId("local"), "team-a")

        assertEquals(1, pools.size)
        with(pools.single()) {
            assertEquals("local", cluster)
            assertEquals("team-a", namespace)
            assertEquals("scanner", name)
            assertEquals(3L, generation)
            assertEquals("exec", executionMode)
            assertEquals("warmLease", lifecycleProfile)
            assertEquals(listOf(2, 1, 1, 0, 0), listOf(capacity.desired, capacity.starting, capacity.ready, capacity.leased, capacity.terminating))
            assertEquals("Ready", conditions.single().type)
            assertEquals("CapacityReady", conditions.single().reason)
        }
        client.close()
        Unit
    }

    @Test
    fun mapsTaskRunDetailAndReturnsNullForMissingResource() = runBlocking {
        server.expect().get()
            .withPath("/apis/kember.openflood.org/v1alpha1/namespaces/team-a/taskruns/scan")
            .andReturn(200, TASK_RUN)
            .once()
        server.expect().get()
            .withPath("/apis/kember.openflood.org/v1alpha1/namespaces/team-a/taskruns/missing")
            .andReturn(404, "")
            .once()
        val client = server.createClient()
        val repository = Fabric8KemberResourceRepository(client, Dispatchers.IO)

        val taskRun = repository.getTaskRun(ResourceRef(ClusterId("local"), "team-a", "scan"))!!

        assertEquals("scanner", taskRun.workerPool)
        assertEquals("Running", taskRun.phase)
        assertEquals("scanner-abc", taskRun.assignedWorker)
        assertEquals("2026-07-18T01:00:01.123Z", taskRun.dispatchedAt)
        assertEquals("Assigned", taskRun.conditions.single().reason)
        assertNull(repository.getTaskRun(ResourceRef(ClusterId("local"), "team-a", "missing")))
        client.close()
        Unit
    }
}

private const val WORKER_POOL_LIST = """
{
  "apiVersion":"kember.openflood.org/v1alpha1",
  "kind":"WorkerPoolList",
  "items":[{
    "apiVersion":"kember.openflood.org/v1alpha1",
    "kind":"WorkerPool",
    "metadata":{"name":"scanner","namespace":"team-a","generation":3},
    "spec":{"execution":{"mode":"exec"},"lifecycle":{"profile":"warmLease"}},
    "status":{
      "capacity":{"desired":2,"starting":1,"ready":1,"leased":0,"terminating":0},
      "conditions":[{"type":"Ready","status":"True","reason":"CapacityReady","message":"ready","lastTransitionTime":"2026-07-18T01:00:00Z"}]
    }
  }]
}
"""

private const val TASK_RUN = """
{
  "apiVersion":"kember.openflood.org/v1alpha1",
  "kind":"TaskRun",
  "metadata":{"name":"scan","namespace":"team-a","creationTimestamp":"2026-07-18T01:00:00Z"},
  "spec":{"workerPoolRef":{"name":"scanner"}},
  "status":{
    "phase":"Running",
    "workerRef":{"name":"scanner-abc"},
    "dispatchedAt":"2026-07-18T01:00:01.123Z",
    "conditions":[{"type":"Dispatched","status":"True","reason":"Assigned","message":"assigned","lastTransitionTime":"2026-07-18T01:00:01Z"}]
  }
}
"""
