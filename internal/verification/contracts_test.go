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
	for name, want := range map[string]State{"passed": Passed, "duplicate-result": Error, "missing-result": Error} {
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
			if receipt.Version != ReceiptVersion || receipt.Identity.Head == receipt.Identity.Tree {
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
	for name, value := range map[string]any{"receipt": Receipt{}, "identity": Identity{}, "fingerprints": Fingerprints{}, "check": Check{}, "result": Result{}} {
		t.Run(name, func(t *testing.T) {
			definition := schema
			if name != "receipt" {
				definition = definitions[name].(map[string]any)
			}
			properties := definition["properties"].(map[string]any)
			var keys, required, fields []string
			for key := range properties {
				keys = append(keys, key)
			}
			for _, key := range definition["required"].([]any) {
				required = append(required, key.(string))
			}
			typ := reflect.TypeOf(value)
			for i := 0; i < typ.NumField(); i++ {
				fields = append(fields, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
			}
			slices.Sort(keys)
			slices.Sort(required)
			slices.Sort(fields)
			if !slices.Equal(keys, fields) || !slices.Equal(required, fields) || definition["additionalProperties"] != false {
				t.Fatalf("schema and Go fields disagree: keys %v required %v fields %v", keys, required, fields)
			}
		})
	}
	properties := schema["properties"].(map[string]any)
	if properties["version"].(map[string]any)["const"] != float64(ReceiptVersion) {
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
	identity := definitions["identity"].(map[string]any)["properties"].(map[string]any)
	assertEnum(identity["mode"], ModeMerge, ModeHead)
	assertEnum(identity["executor"], ExecutorAct, ExecutorFlox)
}
