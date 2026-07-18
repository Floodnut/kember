package dev.kember.api.http

import io.ktor.http.HttpStatusCode
import io.ktor.server.application.Application
import io.ktor.server.response.respond
import io.ktor.server.response.respondFile
import io.ktor.server.routing.get
import io.ktor.server.routing.routing
import java.nio.file.Files
import java.nio.file.Path

fun Application.kemberUi(uiDirectory: Path) {
    val root = uiDirectory.toAbsolutePath().normalize()
    val index = root.resolve("index.html")
    require(Files.isDirectory(root) && Files.isRegularFile(index)) {
        "KEMBER_UI_DIR must contain index.html"
    }

    routing {
        get("/{path...}") {
            val requested = call.parameters.getAll("path")?.joinToString("/").orEmpty()
            if (requested == "api" || requested.startsWith("api/")) {
                call.respond(HttpStatusCode.NotFound)
                return@get
            }

            val candidate = root.resolve(requested).normalize()
            val response = if (candidate.startsWith(root) && Files.isRegularFile(candidate)) candidate else index
            call.respondFile(response.toFile())
        }
    }
}
