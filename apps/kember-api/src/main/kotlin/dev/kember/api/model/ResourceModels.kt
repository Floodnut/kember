package dev.kember.api.model

@JvmInline
value class ClusterId(val value: String)

data class ResourceRef(
    val cluster: ClusterId,
    val namespace: String,
    val name: String,
)
