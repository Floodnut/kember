package dev.kember.api.repository

import dev.kember.api.model.ClusterId
import dev.kember.api.model.ResourceRef
import dev.kember.api.model.TaskRunView
import dev.kember.api.model.WorkerPoolView

interface KemberResourceRepository {
    suspend fun listWorkerPools(cluster: ClusterId, namespace: String): List<WorkerPoolView>
    suspend fun getWorkerPool(ref: ResourceRef): WorkerPoolView?
    suspend fun listTaskRuns(cluster: ClusterId, namespace: String): List<TaskRunView>
    suspend fun getTaskRun(ref: ResourceRef): TaskRunView?
}
