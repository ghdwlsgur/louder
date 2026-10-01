package v1alpha1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestGeneratedNotificationPolicySchemaRequiresSupportedEvents(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "config", "crd", "bases", "finops.sre.local_notificationpolicies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	jsonData, err := utilyaml.ToJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	type schemaNode struct {
		Type       string                `json:"type"`
		Items      *schemaNode           `json:"items"`
		Enum       []string              `json:"enum"`
		MinItems   *int                  `json:"minItems"`
		Required   []string              `json:"required"`
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
	properties := definition.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties
	events := properties["events"]
	if events.Type != "array" || events.Items == nil || events.Items.Type != "string" {
		t.Fatalf("spec.events schema = %#v, want array of strings", events)
	}
	want := []string{"DailySummary", "BudgetWarning", "BudgetExceeded", "BudgetThreshold", "CostAnomaly", "CollectionFailed"}
	if !reflect.DeepEqual(events.Items.Enum, want) {
		t.Errorf("spec.events enum = %#v, want %#v", events.Items.Enum, want)
	}
	if events.MinItems == nil || *events.MinItems != 1 {
		t.Errorf("spec.events minItems = %v, want 1", events.MinItems)
	}
	if !contains(definition.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Required, "events") {
		t.Error("spec.events must be required")
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestNotificationPolicyDeepCopyClonesEvents(t *testing.T) {
	original := &NotificationPolicy{Spec: NotificationPolicySpec{Events: []string{"BudgetThreshold"}}}
	cloned := original.DeepCopy()
	cloned.Spec.Events[0] = "DailySummary"
	if original.Spec.Events[0] != "BudgetThreshold" {
		t.Fatalf("mutating copied events changed original to %q", original.Spec.Events[0])
	}
}

func TestGeneratedBudgetPolicySchemaIncludesOptionalSchedule(t *testing.T) {
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
		Format     string                `json:"format"`
		Items      *schemaNode           `json:"items"`
		MinLength  *int                  `json:"minLength"`
		Required   []string              `json:"required"`
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
	crdRoot := definition.Spec.Versions[0].Schema.OpenAPIV3Schema
	root := crdRoot.Properties["spec"]
	schedule := root.Properties["schedule"]
	if schedule.Type != "string" || schedule.MinLength == nil || *schedule.MinLength != 1 {
		t.Errorf("spec.schedule schema = %#v, want non-empty string", schedule)
	}
	if contains(root.Required, "schedule") {
		t.Error("spec.schedule must remain optional so existing BudgetPolicies do not start recurring")
	}
	status := crdRoot.Properties["status"]
	if status.Properties["lastNotifiedMonth"].Type != "string" {
		t.Errorf("status.lastNotifiedMonth schema = %#v, want string", status.Properties["lastNotifiedMonth"])
	}
	thresholds := status.Properties["notifiedThresholds"]
	if thresholds.Type != "array" || thresholds.Items == nil || thresholds.Items.Type != "integer" || thresholds.Items.Format != "int32" {
		t.Errorf("status.notifiedThresholds schema = %#v, want int32 array", thresholds)
	}
}

func TestBudgetPolicyDeepCopyClonesNotifiedThresholds(t *testing.T) {
	original := &BudgetPolicy{Status: BudgetPolicyStatus{LastNotifiedMonth: "2026-10", NotifiedThresholds: []int32{80}}}
	cloned := original.DeepCopy()
	cloned.Status.NotifiedThresholds[0] = 100
	if original.Status.NotifiedThresholds[0] != 80 {
		t.Fatalf("mutating copied thresholds changed original to %d", original.Status.NotifiedThresholds[0])
	}
}
