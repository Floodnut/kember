package dev.kember.api

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.nio.file.Path

class ConfigurationTest {
    @Test
    fun usesDefaultPort() {
        val config = ApiConfig.from(mapOf(
            "KEMBER_NAMESPACE" to "team-a",
            "KEMBER_UI_DIR" to "/opt/kember/ui",
        ))
        assertEquals(8080, config.port)
        assertEquals(Path.of("/opt/kember/ui"), config.uiDirectory)
    }

    @Test
    fun rejectsMissingNamespaceAndInvalidPort() {
        assertThrows(IllegalArgumentException::class.java) { ApiConfig.from(emptyMap()) }
        assertThrows(IllegalArgumentException::class.java) {
            ApiConfig.from(mapOf("KEMBER_NAMESPACE" to "team-a"))
        }
        assertThrows(IllegalArgumentException::class.java) {
            ApiConfig.from(mapOf(
                "KEMBER_NAMESPACE" to "team-a", "KEMBER_UI_DIR" to "/tmp/ui", "KEMBER_API_PORT" to "abc",
            ))
        }
        assertThrows(IllegalArgumentException::class.java) {
            ApiConfig.from(mapOf(
                "KEMBER_NAMESPACE" to "team-a", "KEMBER_UI_DIR" to "/tmp/ui", "KEMBER_API_PORT" to "70000",
            ))
        }
    }
}
