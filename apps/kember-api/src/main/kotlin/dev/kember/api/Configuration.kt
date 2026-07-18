package dev.kember.api

data class ApiConfig(val namespace: String, val port: Int) {
    companion object {
        fun from(environment: Map<String, String>): ApiConfig {
            val namespace = environment["KEMBER_NAMESPACE"]
                ?: throw IllegalArgumentException("KEMBER_NAMESPACE is required")
            val rawPort = environment["KEMBER_API_PORT"]
            val port = if (rawPort == null) {
                8080
            } else {
                rawPort.toIntOrNull()
                    ?: throw IllegalArgumentException("KEMBER_API_PORT must be an integer")
            }
            require(port in 1..65535) { "KEMBER_API_PORT must be between 1 and 65535" }
            return ApiConfig(namespace, port)
        }
    }
}
