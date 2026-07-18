package org.openflood.kember.plugin.v1

private const val POLICY_ALLOWED_WIRE_HEX =
    "0801120e504f4c4943595f414c4c4f5745441a07616c6c6f77656422027631"

fun main() {
    val response = PolicyResponse.newBuilder()
        .setDecision(PolicyDecisionValue.POLICY_DECISION_ALLOW)
        .setReasonCode("POLICY_ALLOWED")
        .setReason("allowed")
        .setPolicyRevision("v1")
        .build()

    val encoded = response.toByteArray().joinToString("") { "%02x".format(it) }

    check(encoded == POLICY_ALLOWED_WIRE_HEX) {
        "wire encoding = $encoded, want $POLICY_ALLOWED_WIRE_HEX"
    }
}
