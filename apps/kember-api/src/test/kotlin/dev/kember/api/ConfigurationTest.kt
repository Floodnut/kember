package dev.kember.api

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class ConfigurationTest {
    @Test
    fun usesDefaultPort() {
        assertEquals(8080, ApiConfig.from(mapOf("KEMBER_NAMESPACE" to "team-a")).port)
    }

    @Test
    fun rejectsMissingNamespaceAndInvalidPort() {
        assertThrows(IllegalArgumentException::class.java) { ApiConfig.from(emptyMap()) }
        assertThrows(IllegalArgumentException::class.java) {
            ApiConfig.from(mapOf("KEMBER_NAMESPACE" to "team-a", "KEMBER_API_PORT" to "abc"))
        }
        assertThrows(IllegalArgumentException::class.java) {
            ApiConfig.from(mapOf("KEMBER_NAMESPACE" to "team-a", "KEMBER_API_PORT" to "70000"))
        }
    }
}
