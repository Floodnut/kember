package pluginv1_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type policyFixture struct {
	Response        *policyResponse `json:"response"`
	TransportError  string          `json:"transportError"`
	FailurePolicy   string          `json:"failurePolicy"`
	ExpectedOutcome string          `json:"expectedOutcome"`
}

type policyResponse struct {
	Decision       string `json:"decision"`
	ReasonCode     string `json:"reasonCode"`
	Reason         string `json:"reason"`
	PolicyRevision string `json:"policyRevision"`
}

type fakePlugin struct {
	response *policyResponse
	err      error
}

func (p fakePlugin) PolicyDecision() (*policyResponse, error) {
	return p.response, p.err
}

func TestPolicyContractKeepsStableWireSurface(t *testing.T) {
	schema := readContractFile(t, "plugin.proto")

	required := []string{
		`syntax = "proto3";`,
		`package kember.plugin.v1;`,
		`service Plugin`,
		`rpc PolicyDecision(PolicyRequest) returns (PolicyResponse);`,
		`POLICY_DECISION_UNSPECIFIED = 0;`,
		`POLICY_DECISION_ALLOW = 1;`,
		`POLICY_DECISION_DENY = 2;`,
		`POLICY_DECISION_ABSTAIN = 3;`,
	}
	for _, declaration := range required {
		if !strings.Contains(schema, declaration) {
			t.Errorf("plugin contract must contain %q", declaration)
		}
	}

	requiredFields := []string{
		`string protocol_version = 1;`,
		`string request_id = 2;`,
		`int64 deadline_unix_millis = 3;`,
		`string caller_subject = 4;`,
		`string namespace = 5;`,
		`WorkerPoolSnapshot worker_pool = 6;`,
		`TaskRunSnapshot task_run = 7;`,
		`string image_digest = 8;`,
		`string cluster = 9;`,
		`PolicyDecisionValue decision = 1;`,
		`string reason_code = 2;`,
		`string reason = 3;`,
		`string policy_revision = 4;`,
	}
	for _, field := range requiredFields {
		if !strings.Contains(schema, field) {
			t.Errorf("plugin contract must contain stable field %q", field)
		}
	}
}

func TestPolicyContractDoesNotCarryDataPlaneCredentialsOrPayloads(t *testing.T) {
	schema := strings.ToLower(readContractFile(t, "plugin.proto"))

	forbiddenField := regexp.MustCompile(`(?m)^\s*(?:bytes|string)\s+(secret|token|input_data|payload)\s*=`)
	if match := forbiddenField.FindString(schema); match != "" {
		t.Fatalf("plugin contract exposes forbidden data-plane field %q", strings.TrimSpace(match))
	}
}

func TestFakePluginDecisionFixtures(t *testing.T) {
	fixtures := []string{
		"policy-allow.json",
		"policy-deny.json",
		"policy-abstain.json",
		"transport-error-deny.json",
		"transport-error-allow.json",
	}

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			var fixture policyFixture
			if err := json.Unmarshal([]byte(readTestdataFile(t, name)), &fixture); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}

			plugin := fakePlugin{response: fixture.Response}
			if fixture.TransportError != "" {
				plugin.err = errors.New(fixture.TransportError)
			}
			response, err := plugin.PolicyDecision()
			if got := policyOutcome(response, err, fixture.FailurePolicy); got != fixture.ExpectedOutcome {
				t.Fatalf("outcome = %q, want %q", got, fixture.ExpectedOutcome)
			}
		})
	}
}

func policyOutcome(response *policyResponse, err error, failurePolicy string) string {
	if err != nil {
		if failurePolicy == "allow" {
			return "continue_with_audit"
		}
		return "rejected"
	}

	switch response.Decision {
	case "ALLOW":
		return "continue"
	case "DENY":
		return "rejected"
	case "ABSTAIN":
		return "delegate_to_core_policy"
	default:
		return "invalid_response"
	}
}

func readContractFile(t *testing.T, name string) string {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test source")
	}
	paths := []string{filepath.Join(filepath.Dir(sourceFile), name)}
	if testSrcDir := os.Getenv("TEST_SRCDIR"); testSrcDir != "" {
		workspace := os.Getenv("TEST_WORKSPACE")
		paths = append(paths, filepath.Join(testSrcDir, workspace, "packages/worker-contract/proto/kember/plugin/v1", name))
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err == nil {
			return string(content)
		}
	}
	t.Fatalf("read contract file %q from %v", name, paths)
	return ""
}

func readTestdataFile(t *testing.T, name string) string {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test source")
	}
	paths := []string{filepath.Join(filepath.Dir(sourceFile), "../../../../testdata", name)}
	if testSrcDir := os.Getenv("TEST_SRCDIR"); testSrcDir != "" {
		workspace := os.Getenv("TEST_WORKSPACE")
		paths = append(paths, filepath.Join(testSrcDir, workspace, "packages/worker-contract/testdata", name))
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err == nil {
			return string(content)
		}
	}
	t.Fatalf("read testdata file %q from %v", name, paths)
	return ""
}
