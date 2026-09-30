package v1alpha1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestGeneratedCRDsUseFinOpsAPIVersion(t *testing.T) {
	dir := filepath.Join("..", "..", "config", "crd", "bases")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	type crdSpec struct {
		Group string `json:"group"`
		Names struct {
			Kind string `json:"kind"`
		} `json:"names"`
		Versions []struct {
			Name string `json:"name"`
		} `json:"versions"`
	}
	type crd struct {
		Spec crdSpec `json:"spec"`
	}

	found := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		jsonData, err := utilyaml.ToJSON(content)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		var definition crd
		if err := json.Unmarshal(jsonData, &definition); err != nil {
			t.Fatalf("decode %s: %v", entry.Name(), err)
		}
		if definition.Spec.Names.Kind == "" {
			continue
		}
		found[definition.Spec.Names.Kind] = true
		if definition.Spec.Group != "finops.sre.local" {
			t.Errorf("%s group = %q, want finops.sre.local", definition.Spec.Names.Kind, definition.Spec.Group)
		}
		if len(definition.Spec.Versions) != 1 || definition.Spec.Versions[0].Name != "v1alpha1" {
			t.Errorf("%s versions = %#v, want [v1alpha1]", definition.Spec.Names.Kind, definition.Spec.Versions)
		}
	}

	for _, kind := range []string{"CloudAccount", "BudgetPolicy", "NotificationPolicy"} {
		if !found[kind] {
			t.Errorf("generated CRD for %s was not found", kind)
		}
	}
}

func TestGeneratedBudgetPolicySchemaMatchesArchitecture(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "finops.sre.local_budgetpolicies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	jsonData, err := utilyaml.ToJSON(content)
	if err != nil {
		t.Fatal(err)
	}

	type schemaNode struct {
		Type       string                `json:"type"`
		Properties map[string]schemaNode `json:"properties"`
	}
	type schema struct {
		Spec struct {
			Versions []struct {
				Schema struct {
					OpenAPIV3Schema schemaNode `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	var definition schema
	if err := json.Unmarshal(jsonData, &definition); err != nil {
		t.Fatal(err)
	}
	if len(definition.Spec.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(definition.Spec.Versions))
	}

	properties := definition.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties
	if got := properties["amount"].Properties["value"].Type; got != "integer" {
		t.Errorf("spec.amount.value type = %q, want integer", got)
	}
	forecast := properties["forecast"]
	if forecast.Type != "object" || forecast.Properties["enabled"].Type != "boolean" {
		t.Errorf("spec.forecast schema = %#v, want object with boolean enabled", forecast)
	}
}
