// Package receipt validates evidence content, not its producer or provenance.
package receipt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	v "github.com/smeltery/bellows/internal/verification"
)

const MaxBytes = 1 << 20

func Decode(data []byte) (v.Receipt, error) {
	var r v.Receipt
	if !utf8.Valid(data) {
		return r, fmt.Errorf("receipt contains invalid UTF-8")
	}
	if len(data) > MaxBytes {
		return r, fmt.Errorf("receipt exceeds %d bytes", MaxBytes)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	actual, err := readValue(d, 0)
	if err != nil {
		return r, err
	}
	if _, err = d.Token(); err != io.EOF {
		return r, fmt.Errorf("receipt contains trailing data")
	}
	if err = json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	canonical, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	var expected any
	if err = json.Unmarshal(canonical, &expected); err != nil {
		return r, err
	}
	if !sameShape(actual, expected) {
		return r, fmt.Errorf("receipt has unknown, missing or null fields")
	}
	object := actual.(map[string]any)
	for _, key := range []string{"started_at", "completed_at"} {
		if value, ok := object[key].(string); ok && !strings.HasSuffix(value, "Z") {
			return r, fmt.Errorf("%s must use UTC Z notation", key)
		}
	}
	for _, raw := range object["results"].([]any) {
		if provenance, ok := raw.(map[string]any)["reuse"].(map[string]any); ok {
			for _, key := range []string{"completed_at", "expires_at"} {
				if value, ok := provenance[key].(string); ok && !strings.HasSuffix(value, "Z") {
					return r, fmt.Errorf("reuse %s must use UTC Z notation", key)
				}
			}
		}
	}
	return r, Validate(r)
}

func readValue(d *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, fmt.Errorf("receipt nesting is too deep")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key")
			}
			if _, exists := object[name]; exists {
				return nil, fmt.Errorf("duplicate receipt field")
			}
			object[name], err = readValue(d, depth+1)
			if err != nil {
				return nil, err
			}
		}
		_, err = d.Token()
		return object, err
	case json.Delim('['):
		array := []any{}
		for d.More() {
			value, err := readValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err = d.Token()
		return array, err
	}
	return token, nil
}

func sameShape(actual, expected any) bool {
	switch expected := expected.(type) {
	case map[string]any:
		object, ok := actual.(map[string]any)
		if !ok || len(object) != len(expected) {
			return false
		}
		for key, value := range expected {
			found, exists := object[key]
			if !exists || !sameShape(found, value) {
				return false
			}
		}
	case []any:
		array, ok := actual.([]any)
		if !ok || len(array) != len(expected) {
			return false
		}
		for i := range expected {
			if !sameShape(array[i], expected[i]) {
				return false
			}
		}
	default:
		return (actual == nil) == (expected == nil)
	}
	return true
}

func Encode(r v.Receipt) ([]byte, error) {
	if err := Validate(r); err != nil {
		return nil, err
	}
	r.Checks, r.Results = slices.Clone(r.Checks), slices.Clone(r.Results)
	slices.SortFunc(r.Checks, func(a, b v.Check) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(r.Results, func(a, b v.Result) int { return strings.Compare(a.ID, b.ID) })
	for i := range r.Checks {
		r.Checks[i].Needs = slices.Clone(r.Checks[i].Needs)
		slices.Sort(r.Checks[i].Needs)
	}
	data, err := json.Marshal(r)
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("receipt exceeds %d bytes", MaxBytes)
	}
	return data, err
}
