package verification

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEvaluateRequiredChecks(t *testing.T) {
	zero, nonzero := 0, 1
	check := Check{ID: "test", Required: true, Needs: []string{}}
	passed := Result{ID: "test", State: Passed, ExitCode: &zero}
	type testCase struct {
		name    string
		input   InputState
		checks  []Check
		results []Result
		want    State
	}
	tests := []testCase{
		{"passed", Ready, []Check{check}, []Result{passed}, Passed},
		{"missing", Ready, []Check{check}, nil, Error},
		{"duplicate", Ready, []Check{check}, []Result{passed, passed}, Error},
		{"duplicate check", Ready, []Check{check, check}, []Result{passed}, Error},
		{"empty id", Ready, []Check{{Required: true}}, nil, Error},
		{"unexpected result", Ready, []Check{check}, []Result{{ID: "other", State: Passed, ExitCode: &zero}}, Error},
		{"unknown state", Ready, []Check{check}, []Result{{ID: "test", State: "unknown"}}, Error},
		{"missing exit", Ready, []Check{check}, []Result{{ID: "test", State: Passed}}, Error},
		{"nonzero exit", Ready, []Check{check}, []Result{{ID: "test", State: Passed, ExitCode: &nonzero}}, Error},
		{"negative duration", Ready, []Check{check}, []Result{{ID: "test", State: Passed, ExitCode: &zero, DurationMS: -1}}, Error},
		{"no required checks", Ready, []Check{{ID: "test"}}, []Result{passed}, Unsupported},
		{"no checks", Ready, nil, nil, Unsupported},
		{"missing prerequisite", Ready, []Check{{ID: "test", Required: true, Needs: []string{"build"}}}, []Result{passed}, Error},
		{"passed prerequisite", Ready, []Check{check, {ID: "integration", Required: true, Needs: []string{"test"}}}, []Result{passed, {ID: "integration", State: Passed, ExitCode: &zero}}, Passed},
		{"failed transitive prerequisite", Ready, []Check{{ID: "test", Required: true, Needs: []string{"optional"}}, {ID: "optional", Needs: []string{"build"}}, {ID: "build"}}, []Result{passed, {ID: "optional", State: Passed, ExitCode: &zero}, {ID: "build", State: Failed, ExitCode: &nonzero}}, Error},
		{"optional failure", Ready, []Check{check, {ID: "optional"}}, []Result{passed, {ID: "optional", State: Failed, ExitCode: &nonzero}}, Passed},
	}
	for _, input := range []InputState{"", Dirty, Conflict, Stale, Invalid, "unknown"} {
		tests = append(tests, testCase{"input " + string(input), input, []Check{check}, []Result{passed}, Error})
	}
	for _, state := range []State{Pending, Failed, Skipped, Cancelled, Unsupported, Error} {
		t.Run(string(state), func(t *testing.T) {
			if got := Evaluate(Ready, []Check{check}, []Result{{ID: "test", State: state}}); got != state {
				t.Fatalf("Evaluate = %q; want %q", got, state)
			}
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Evaluate(tt.input, tt.checks, tt.results); got != tt.want {
				t.Fatalf("Evaluate = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestReceiptFixtures(t *testing.T) {
	for name, want := range map[string]State{"passed": Passed, "duplicate-result": Error, "missing-result": Error, "conflict": Error} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var receipt Receipt
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.Version != 1 || receipt.Identity.Head == receipt.Identity.Tree {
				t.Fatal("fixture must identify a v1 proposed merge tree separately from its head")
			}
			if got := Evaluate(receipt.Input, receipt.Checks, receipt.Results); got != want {
				t.Fatalf("Evaluate = %q; want %q", got, want)
			}
			encoded, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			var before, after any
			if err := json.Unmarshal(data, &before); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &after); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("fixture changed during Go serialization")
			}
		})
	}
}

func TestReceiptSchemaMatchesContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "schemas", "verification-receipt.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	definitions := schema["$defs"].(map[string]any)
	for name, value := range map[string]any{"receipt": Receipt{}, "identity": Identity{}, "fingerprints": Fingerprints{}, "check": Check{}, "result": Result{}, "reuse": Reuse{}} {
		t.Run(name, func(t *testing.T) {
			definition := schema
			if name != "receipt" {
				definition = definitions[name].(map[string]any)
			}
			properties := definition["properties"].(map[string]any)
			var keys, required, fields, mandatory []string
			for key := range properties {
				keys = append(keys, key)
			}
			for _, key := range definition["required"].([]any) {
				required = append(required, key.(string))
			}
			typ := reflect.TypeOf(value)
			for i := 0; i < typ.NumField(); i++ {
				tag := typ.Field(i).Tag.Get("json")
				field := strings.Split(tag, ",")[0]
				fields = append(fields, field)
				if !strings.Contains(tag, "omitempty") {
					mandatory = append(mandatory, field)
				}
			}
			slices.Sort(keys)
			slices.Sort(required)
			slices.Sort(fields)
			slices.Sort(mandatory)
			if !slices.Equal(keys, fields) || !slices.Equal(required, mandatory) || definition["additionalProperties"] != false {
				t.Fatalf("schema and Go fields disagree: keys %v required %v fields %v", keys, required, fields)
			}
		})
	}
	properties := schema["properties"].(map[string]any)
	if !reflect.DeepEqual(properties["version"].(map[string]any)["enum"], []any{float64(1), float64(ReceiptVersion)}) {
		t.Fatal("schema version differs")
	}
	assertEnum := func(property any, values ...string) {
		t.Helper()
		var actual []string
		for _, value := range property.(map[string]any)["enum"].([]any) {
			actual = append(actual, value.(string))
		}
		if !slices.Equal(actual, values) {
			t.Fatalf("enum %v differs from %v", actual, values)
		}
	}
	assertEnum(properties["input"], string(Ready), string(Dirty), string(Conflict), string(Stale), string(Invalid))
	result := definitions["result"].(map[string]any)["properties"].(map[string]any)
	assertEnum(result["state"], string(Pending), string(Passed), string(Failed), string(Skipped), string(Cancelled), string(Unsupported), string(Error))
	identity := definitions["identity_ready"].(map[string]any)["properties"].(map[string]any)
	assertEnum(identity["mode"], ModeMerge, ModeHead)
	assertEnum(identity["executor"], ExecutorAct, ExecutorFlox)
}

func TestReadySchemaRequiresResolvedIdentity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "schemas", "verification-receipt.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	rule := schema["allOf"].([]any)[0].(map[string]any)
	condition := rule["if"].(map[string]any)["properties"].(map[string]any)["input"].(map[string]any)
	target := rule["then"].(map[string]any)["properties"].(map[string]any)["identity"].(map[string]any)
	if condition["const"] != string(Ready) || target["$ref"] != "#/$defs/identity_ready" {
		t.Fatal("ready identity must select strict schema")
	}
	defs := schema["$defs"].(map[string]any)
	for _, name := range []string{"identity", "fingerprints"} {
		permissive := defs[name].(map[string]any)["properties"].(map[string]any)
		strict := defs[name+"_ready"].(map[string]any)["properties"].(map[string]any)
		for field, value := range permissive {
			pattern, exists := value.(map[string]any)["pattern"].(string)
			if !exists {
				continue
			}
			strictPattern := strict[field].(map[string]any)["pattern"].(string)
			if pattern != "^$|"+strictPattern {
				t.Fatalf("%s.%s must permit absence only outside ready state", name, field)
			}
		}
	}
}

func TestEvaluateOrderIndependent(t *testing.T) {
	checks := []Check{{ID: "a", Required: true}, {ID: "b", Required: true}}
	states := []State{Pending, Skipped, Unsupported, Cancelled, Failed, Error}
	for i, first := range states {
		for j, second := range states {
			results := []Result{{ID: "a", State: first}, {ID: "b", State: second}}
			want := first
			if j > i {
				want = second
			}
			if got := Evaluate(Ready, checks, results); got != want {
				t.Fatalf("%s/%s: %s want %s", first, second, got, want)
			}
			reversed := []Check{checks[1], checks[0]}
			if got := Evaluate(Ready, reversed, results); got != want {
				t.Fatalf("reordering changed result: %s", got)
			}
		}
	}
}
