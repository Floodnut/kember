package v1alpha1

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"sigs.k8s.io/yaml"
)

func loadCRD(t *testing.T, filename string) map[string]any {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "deploy", "crd", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var crd map[string]any
	if err := yaml.Unmarshal(data, &crd); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return crd
}

func nestedMap(t *testing.T, value any, keys ...string) map[string]any {
	t.Helper()
	current, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%v is not an object", keys)
	}
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("%v is not an object", append(keys[:0:0], key))
		}
		current = next
	}
	return current
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("%v is not an array", value)
	}
	result := make([]string, len(items))
	for index, item := range items {
		result[index], ok = item.(string)
		if !ok {
			t.Fatalf("array item %d is not a string", index)
		}
	}
	return result
}

func TestCRDsUseNamespacedV1alpha1API(t *testing.T) {
	tests := []struct {
		filename string
		kind     string
		plural   string
	}{
		{filename: "kember.dev_workerpools.yaml", kind: "WorkerPool", plural: "workerpools"},
		{filename: "kember.dev_taskruns.yaml", kind: "TaskRun", plural: "taskruns"},
	}

	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			crd := loadCRD(t, test.filename)
			spec := nestedMap(t, crd["spec"])
			if spec["group"] != "kember.dev" || spec["scope"] != "Namespaced" {
				t.Fatalf("unexpected API identity: group=%v scope=%v", spec["group"], spec["scope"])
			}
			names := nestedMap(t, spec["names"])
			if names["kind"] != test.kind || names["plural"] != test.plural {
				t.Fatalf("unexpected names: kind=%v plural=%v", names["kind"], names["plural"])
			}
			versions, ok := spec["versions"].([]any)
			if !ok || len(versions) != 1 {
				t.Fatalf("expected one served version, got %v", spec["versions"])
			}
			version := nestedMap(t, versions[0])
			if version["name"] != "v1alpha1" || version["storage"] != true {
				t.Fatalf("unexpected version: %v", version)
			}
			if _, ok := version["subresources"].(map[string]any); !ok {
				t.Fatal("status subresource is missing")
			}
		})
	}
}

func TestWorkerPoolExecutionContractIsExplicit(t *testing.T) {
	crd := loadCRD(t, "kember.dev_workerpools.yaml")
	versions := nestedMap(t, crd["spec"])["versions"].([]any)
	version := nestedMap(t, versions[0])
	schema := nestedMap(t, version["schema"], "openAPIV3Schema")
	properties := nestedMap(t, schema["properties"])
	specSchema := nestedMap(t, properties["spec"])
	specProperties := nestedMap(t, specSchema["properties"])
	execution := nestedMap(t, specProperties["execution"], "properties")
	lifecycle := nestedMap(t, specProperties["lifecycle"], "properties")
	if got := stringSlice(t, nestedMap(t, execution["mode"])["enum"]); !equalStrings(got, []string{"job", "exec"}) {
		t.Fatalf("unexpected execution modes: %v", got)
	}
	if got := stringSlice(t, nestedMap(t, lifecycle["profile"])["enum"]); !equalStrings(got, []string{"runToCompletion", "warmLease"}) {
		t.Fatalf("unexpected lifecycle profiles: %v", got)
	}
	if _, ok := specSchema["x-kubernetes-validations"].([]any); !ok {
		t.Fatal("execution/lifecycle compatibility validation is missing")
	}
}

func TestTaskRunPhaseAndInputContractAreExplicit(t *testing.T) {
	crd := loadCRD(t, "kember.dev_taskruns.yaml")
	versions := nestedMap(t, crd["spec"])["versions"].([]any)
	version := nestedMap(t, versions[0])
	schema := nestedMap(t, version["schema"], "openAPIV3Schema")
	properties := nestedMap(t, schema["properties"])
	specProperties := nestedMap(t, properties["spec"], "properties")
	inputProperties := nestedMap(t, specProperties["input"], "properties")
	inputRef := nestedMap(t, inputProperties["ref"])
	if inputRef["pattern"] != "^[A-Za-z][A-Za-z0-9+.-]*://.+$" {
		t.Fatalf("unexpected input.ref pattern: %v", inputRef["pattern"])
	}
	statusProperties := nestedMap(t, properties["status"], "properties")
	phase := nestedMap(t, statusProperties["phase"])
	expected := []string{"Pending", "Running", "Succeeded", "Failed", "TimedOut", "Rejected", "Cancelled"}
	if got := stringSlice(t, phase["enum"]); !equalStrings(got, expected) {
		t.Fatalf("unexpected phases: %v", got)
	}
	for _, field := range []string{"resolvedTemplate", "jobRef", "workerRef", "dispatchedAt", "completedAt"} {
		if _, ok := statusProperties[field]; !ok {
			t.Fatalf("status.%s is missing", field)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
