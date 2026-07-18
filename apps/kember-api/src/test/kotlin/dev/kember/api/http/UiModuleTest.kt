package dev.kember.api.http

import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import io.ktor.http.ContentType
import io.ktor.http.HttpStatusCode
import io.ktor.http.contentType
import io.ktor.server.testing.testApplication
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files

class UiModuleTest {
    @Test
    fun servesAssetsAndFallsBackToTheSpaForUiRoutes() {
        val root = Files.createTempDirectory("kember-ui-test")
        Files.createDirectories(root.resolve("assets"))
        Files.writeString(root.resolve("index.html"), "<html>Kember UI</html>")
        Files.writeString(root.resolve("assets/app.js"), "console.log('kember')")

        testApplication {
            application { kemberUi(root) }

            val rootResponse = client.get("/")
            assertEquals(HttpStatusCode.OK, rootResponse.status)
            assertTrue(rootResponse.bodyAsText().contains("Kember UI"))

            val routeResponse = client.get("/task-runs/example")
            assertEquals(HttpStatusCode.OK, routeResponse.status)
            assertTrue(routeResponse.bodyAsText().contains("Kember UI"))

            val assetResponse = client.get("/assets/app.js")
            assertEquals(HttpStatusCode.OK, assetResponse.status)
            assertEquals(ContentType.parse("text/javascript"), assetResponse.contentType()?.withoutParameters())
            assertTrue(assetResponse.bodyAsText().contains("kember"))
        }
    }

    @Test
    fun doesNotTurnUnknownApiRoutesIntoSpaResponses() = testApplication {
        val root = Files.createTempDirectory("kember-ui-test")
        Files.writeString(root.resolve("index.html"), "<html>Kember UI</html>")
        application { kemberUi(root) }

        assertEquals(HttpStatusCode.NotFound, client.get("/api/v1/unknown").status)
    }
}
