package dev.kember.api.policy

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class NamespaceAccessPolicyTest {
    @Test
    fun exposesOnlyConfiguredNamespace() {
        val policy = SingleNamespaceAccessPolicy("team-a")

        assertEquals(setOf("team-a"), policy.visibleNamespaces())
        assertTrue(policy.allows("team-a"))
        assertFalse(policy.allows("team-b"))
    }

    @Test
    fun rejectsInvalidNamespace() {
        val invalid = listOf(
            "",
            "Team-a",
            "team_a",
            "-team-a",
            "team-a-",
            "a".repeat(64),
        )

        invalid.forEach { namespace ->
            assertThrows(IllegalArgumentException::class.java) {
                SingleNamespaceAccessPolicy(namespace)
            }
        }
    }
}
