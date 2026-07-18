//go:build bazel

package pluginv1_test

import (
	"encoding/hex"
	"testing"

	pluginv1 "github.com/Floodnut/kember/packages/worker-contract/gen/kember/plugin/v1"
	"google.golang.org/protobuf/proto"
)

const policyAllowedWireHex = "0801120e504f4c4943595f414c4c4f5745441a07616c6c6f77656422027631"

func TestGeneratedGoPolicyResponseKeepsCompatibleWireEncoding(t *testing.T) {
	response := &pluginv1.PolicyResponse{
		Decision:       pluginv1.PolicyDecisionValue_POLICY_DECISION_ALLOW,
		ReasonCode:     "POLICY_ALLOWED",
		Reason:         "allowed",
		PolicyRevision: "v1",
	}

	encoded, err := proto.Marshal(response)
	if err != nil {
		t.Fatalf("marshal policy response: %v", err)
	}
	if got := hex.EncodeToString(encoded); got != policyAllowedWireHex {
		t.Fatalf("wire encoding = %q, want %q", got, policyAllowedWireHex)
	}
}
