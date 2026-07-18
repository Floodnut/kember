package dev.kember.api.http

import dev.kember.api.policy.isValidKubernetesName
import dev.kember.api.repository.RepositoryUnavailable
import dev.kember.api.service.KemberQueryService
import dev.kember.api.service.NamespaceNotAllowed
import dev.kember.api.service.ResourceNotFound
import io.ktor.http.HttpStatusCode
import io.ktor.serialization.jackson.jackson
import io.ktor.server.application.Application
import io.ktor.server.application.ApplicationCall
import io.ktor.server.application.install
import io.ktor.server.plugins.contentnegotiation.ContentNegotiation
import io.ktor.server.response.respond
import io.ktor.server.routing.get
import io.ktor.server.routing.route
import io.ktor.server.routing.routing
import kotlinx.coroutines.CancellationException

data class ItemsResponse<T>(val items: List<T>)
data class ErrorBody(val code: String, val message: String)
data class ErrorResponse(val error: ErrorBody)

fun Application.kemberApi(queryService: KemberQueryService) {
    install(ContentNegotiation) {
        jackson()
    }

    routing {
        get("/healthz") {
            call.respond(mapOf("status" to "ok"))
        }
        route("/api/v1") {
            get("/namespaces") {
                call.respond(ItemsResponse(queryService.listNamespaces()))
            }
            route("/namespaces/{namespace}") {
                get("/worker-pools") {
                    call.respondApi {
                        val namespace = call.requiredName("namespace")
                        ItemsResponse(queryService.listWorkerPools(namespace))
                    }
                }
                get("/worker-pools/{name}") {
                    call.respondApi {
                        queryService.getWorkerPool(
                            call.requiredName("namespace"),
                            call.requiredName("name"),
                        )
                    }
                }
                get("/task-runs") {
                    call.respondApi {
                        val namespace = call.requiredName("namespace")
                        ItemsResponse(queryService.listTaskRuns(namespace))
                    }
                }
                get("/task-runs/{name}") {
                    call.respondApi {
                        queryService.getTaskRun(
                            call.requiredName("namespace"),
                            call.requiredName("name"),
                        )
                    }
                }
            }
        }
    }
}

private suspend fun ApplicationCall.respondApi(block: suspend () -> Any) {
    try {
        respond(block())
    } catch (_: InvalidResourceIdentifier) {
        respondError(HttpStatusCode.BadRequest, "invalid_resource_identifier", "resource identifier is invalid")
    } catch (_: NamespaceNotAllowed) {
        respondError(HttpStatusCode.Forbidden, "namespace_not_allowed", "namespace is not available")
    } catch (_: ResourceNotFound) {
        respondError(HttpStatusCode.NotFound, "resource_not_found", "resource was not found")
    } catch (_: RepositoryUnavailable) {
        respondError(HttpStatusCode.ServiceUnavailable, "kubernetes_api_unavailable", "Kubernetes API is unavailable")
    } catch (cancelled: CancellationException) {
        throw cancelled
    } catch (_: Throwable) {
        respondError(HttpStatusCode.InternalServerError, "internal_error", "internal server error")
    }
}

private fun ApplicationCall.requiredName(parameter: String): String {
    val value = parameters[parameter]
    if (value == null || !isValidKubernetesName(value)) {
        throw InvalidResourceIdentifier()
    }
    return value
}

private suspend fun ApplicationCall.respondError(status: HttpStatusCode, code: String, message: String) {
    respond(status, ErrorResponse(ErrorBody(code, message)))
}

private class InvalidResourceIdentifier : RuntimeException()
