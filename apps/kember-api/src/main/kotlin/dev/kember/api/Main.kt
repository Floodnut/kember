package dev.kember.api

import dev.kember.api.http.kemberApi
import dev.kember.api.http.kemberUi
import dev.kember.api.model.ClusterId
import dev.kember.api.policy.SingleNamespaceAccessPolicy
import dev.kember.api.repository.Fabric8KemberResourceRepository
import dev.kember.api.service.KemberQueryService
import io.fabric8.kubernetes.client.KubernetesClientBuilder
import io.ktor.server.engine.embeddedServer
import io.ktor.server.netty.Netty
import kotlinx.coroutines.Dispatchers

fun main() {
    val config = ApiConfig.from(System.getenv())

    val accessPolicy = SingleNamespaceAccessPolicy(config.namespace)
    KubernetesClientBuilder().build().use { client ->
        val queryService = KemberQueryService(
            cluster = ClusterId("local"),
            accessPolicy = accessPolicy,
            repository = Fabric8KemberResourceRepository(client, Dispatchers.IO),
        )
        embeddedServer(Netty, port = config.port) {
            kemberApi(queryService)
            kemberUi(config.uiDirectory)
        }.start(wait = true)
    }
}
