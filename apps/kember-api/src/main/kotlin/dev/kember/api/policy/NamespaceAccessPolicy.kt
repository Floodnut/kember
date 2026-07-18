package dev.kember.api.policy

interface NamespaceAccessPolicy {
    fun visibleNamespaces(): Set<String>
    fun allows(namespace: String): Boolean
}

class SingleNamespaceAccessPolicy(private val namespace: String) : NamespaceAccessPolicy {
    init {
        require(isValidKubernetesName(namespace)) { "invalid Kubernetes namespace" }
    }

    override fun visibleNamespaces(): Set<String> = setOf(namespace)

    override fun allows(namespace: String): Boolean = this.namespace == namespace
}

fun isValidKubernetesName(value: String): Boolean =
    value.length in 1..63 && KUBERNETES_NAME.matches(value)

private val KUBERNETES_NAME = Regex("[a-z0-9]([-a-z0-9]*[a-z0-9])?")
