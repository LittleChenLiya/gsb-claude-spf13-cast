// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

// TestConvertLossyIntegerRejected covers the acceptance scenario:
// items[0].n="12", items[1].n="1.5" converted to int. The second item must
// report the accurate path, no result is published, and the original input
// strings remain unchanged.
func TestConvertLossyIntegerRejected(t *testing.T) {
	input := map[string]any{
		"items": []any{
			map[string]any{"n": "12"},
			map[string]any{"n": "1.5"},
		},
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("items").Index(0).Key("n"), KindInt)
	schema.Set(Path{}.Key("items").Index(1).Key("n"), KindInt)

	result, errs := Convert(input, schema)

	if result != nil {
		t.Fatalf("expected nil result when errors present, got %v", result)
	}

	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}

	err := errs[0]
	wantPath := "items[1].n"
	if err.Path.String() != wantPath {
		t.Errorf("expected error path %q, got %q", wantPath, err.Path.String())
	}

	if !errors.Is(err.Err, errLossy) {
		t.Errorf("expected lossy error, got %v", err.Err)
	}

	// The original input must remain unchanged (still strings).
	items := input["items"].([]any)
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["n"] != "12" {
		t.Errorf("original items[0].n changed: %v", first["n"])
	}
	if second["n"] != "1.5" {
		t.Errorf("original items[1].n changed: %v", second["n"])
	}
}

// TestConvertTwoLegalIntegers covers the acceptance scenario: two legal
// integer strings produce a new object with integer values.
func TestConvertTwoLegalIntegers(t *testing.T) {
	input := map[string]any{
		"items": []any{
			map[string]any{"n": "12"},
			map[string]any{"n": "13"},
		},
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("items").Index(0).Key("n"), KindInt)
	schema.Set(Path{}.Key("items").Index(1).Key("n"), KindInt)

	result, errs := Convert(input, schema)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// The result must be a new object, not the input itself.
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}

	if reflect.ValueOf(resultMap).Pointer() == reflect.ValueOf(input).Pointer() {
		t.Error("result is the same map as input; expected a new object")
	}

	items := resultMap["items"].([]any)
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)

	if first["n"] != 12 {
		t.Errorf("expected items[0].n=12 (int), got %v (%T)", first["n"], first["n"])
	}
	if second["n"] != 13 {
		t.Errorf("expected items[1].n=13 (int), got %v (%T)", second["n"], second["n"])
	}

	// The original input must remain strings.
	origItems := input["items"].([]any)
	origFirst := origItems[0].(map[string]any)
	if origFirst["n"] != "12" {
		t.Errorf("original items[0].n changed: %v", origFirst["n"])
	}
}

// TestConvertOverflowNotTruncatedToZero covers the acceptance scenario:
// an overflow value must be reported as an error and must not be truncated
// to 0.
func TestConvertOverflowNotTruncatedToZero(t *testing.T) {
	input := map[string]any{
		"n": "999",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt8)

	result, errs := Convert(input, schema)

	if result != nil {
		t.Fatalf("expected nil result on overflow, got %v", result)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	if !errors.Is(errs[0].Err, errOverflow) {
		t.Errorf("expected overflow error, got %v", errs[0].Err)
	}

	// The error must not be a truncation to 0.
	if errs[0].Err == nil {
		t.Error("expected non-nil error")
	}

	// The original input must remain unchanged.
	if input["n"] != "999" {
		t.Errorf("original input changed: %v", input["n"])
	}
}

// TestConvertOverflowIntegerValue covers overflow from an actual integer
// value (not a string).
func TestConvertOverflowIntegerValue(t *testing.T) {
	input := map[string]any{
		"n": 1000,
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt8)

	result, errs := Convert(input, schema)

	if result != nil {
		t.Fatalf("expected nil result on overflow, got %v", result)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	if !errors.Is(errs[0].Err, errOverflow) {
		t.Errorf("expected overflow error, got %v", errs[0].Err)
	}
}

// TestConvertNilPreserved covers nil fill: a nil value at a declared path
// is preserved as nil, not converted to 0 or "".
func TestConvertNilPreserved(t *testing.T) {
	input := map[string]any{
		"n": nil,
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(input, schema)

	if len(errs) != 0 {
		t.Fatalf("expected no errors for nil, got %v", errs)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	resultMap := result.(map[string]any)
	if resultMap["n"] != nil {
		t.Errorf("expected nil at n, got %v (%T)", resultMap["n"], resultMap["n"])
	}
}

// TestConvertMissingKey covers missing keys: a declared path that is absent
// from the input is reported as an error.
func TestConvertMissingKey(t *testing.T) {
	input := map[string]any{
		"other": "1",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(input, schema)

	if result != nil {
		t.Fatalf("expected nil result on missing key, got %v", result)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	if !errors.Is(errs[0].Err, errMissingKey) {
		t.Errorf("expected missing key error, got %v", errs[0].Err)
	}

	if errs[0].Path.String() != "n" {
		t.Errorf("expected path n, got %q", errs[0].Path.String())
	}
}

// TestConvertMissingIndex covers missing slice indices.
func TestConvertMissingIndex(t *testing.T) {
	input := map[string]any{
		"items": []any{"1"},
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("items").Index(0), KindInt)
	schema.Set(Path{}.Key("items").Index(5), KindInt)

	result, errs := Convert(input, schema)

	if result != nil {
		t.Fatalf("expected nil result on missing index, got %v", result)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	if !errors.Is(errs[0].Err, errMissingIndex) {
		t.Errorf("expected missing index error, got %v", errs[0].Err)
	}

	if errs[0].Path.String() != "items[5]" {
		t.Errorf("expected path items[5], got %q", errs[0].Path.String())
	}
}

// TestConvertCustomRule covers custom conversion rules via the explicit
// registry.
func TestConvertCustomRule(t *testing.T) {
	input := map[string]any{
		"n": "custom-value",
	}

	schema := NewSchema()
	schema.SetConverter(Path{}.Key("n"), func(v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("expected string")
		}
		return "converted:" + s, nil
	})

	result, errs := Convert(input, schema)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["n"] != "converted:custom-value" {
		t.Errorf("expected custom conversion, got %v", resultMap["n"])
	}
}

// TestConvertCustomRuleError covers a custom converter that returns an error.
func TestConvertCustomRuleError(t *testing.T) {
	input := map[string]any{
		"n": "bad",
	}

	schema := NewSchema()
	schema.SetConverter(Path{}.Key("n"), func(v any) (any, error) {
		return nil, errors.New("custom failure")
	})

	result, errs := Convert(input, schema)

	if result != nil {
		t.Fatalf("expected nil result on custom error, got %v", result)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	if errs[0].Path.String() != "n" {
		t.Errorf("expected path n, got %q", errs[0].Path.String())
	}
}

// TestStrictVsLooseInteger demonstrates the difference between the strict
// converter and the existing loose ToInt functions.
func TestStrictVsLooseInteger(t *testing.T) {
	// Loose ToInt truncates "1.5" to 1 (lossy).
	loose := ToInt("1.5")
	if loose != 1 {
		t.Errorf("loose ToInt(\"1.5\") = %d, expected 1 (truncation)", loose)
	}

	// Strict converter rejects "1.5" as lossy.
	input := map[string]any{"n": "1.5"}
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("strict converter should reject \"1.5\"")
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errLossy) {
		t.Fatalf("expected lossy error, got %v", errs)
	}
}

// TestStrictVsLooseBool demonstrates the difference between strict and
// loose bool conversion.
func TestStrictVsLooseBool(t *testing.T) {
	// Loose ToBool accepts "1" as true (ambiguous).
	loose := ToBool("1")
	if !loose {
		t.Error("loose ToBool(\"1\") should be true")
	}

	// Strict converter rejects "1" as ambiguous.
	input := map[string]any{"n": "1"}
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindBool)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("strict converter should reject \"1\" as bool")
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errAmbiguousBool) {
		t.Fatalf("expected ambiguous bool error, got %v", errs)
	}

	// Strict converter accepts "true".
	input2 := map[string]any{"n": "true"}
	result2, errs2 := Convert(input2, schema)
	if len(errs2) != 0 {
		t.Fatalf("strict converter should accept \"true\", got %v", errs2)
	}
	resultMap := result2.(map[string]any)
	if resultMap["n"] != true {
		t.Errorf("expected true, got %v", resultMap["n"])
	}
}

// TestStrictVsLooseOverflow demonstrates that loose conversion truncates
// while strict conversion reports an error.
func TestStrictVsLooseOverflow(t *testing.T) {
	// Loose ToInt8 truncates 999 to -25 (wrapping).
	loose := ToInt8(999)
	if loose != -25 {
		t.Errorf("loose ToInt8(999) = %d, expected -25 (truncation)", loose)
	}

	// Strict converter rejects 999 as overflow.
	input := map[string]any{"n": 999}
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt8)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("strict converter should reject 999 as int8")
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errOverflow) {
		t.Fatalf("expected overflow error, got %v", errs)
	}
}

// TestConvertCompleteErrorList covers the requirement that the error list
// is complete (all failures reported, not just the first).
func TestConvertCompleteErrorList(t *testing.T) {
	input := map[string]any{
		"a": "1.5", // lossy
		"b": "999", // overflow int8
		"c": "1",   // ambiguous bool
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("a"), KindInt)
	schema.Set(Path{}.Key("b"), KindInt8)
	schema.Set(Path{}.Key("c"), KindBool)

	result, errs := Convert(input, schema)

	if result != nil {
		t.Fatalf("expected nil result with multiple errors, got %v", result)
	}

	if len(errs) != 3 {
		t.Fatalf("expected 3 errors, got %d: %v", len(errs), errs)
	}

	// All three paths should be represented.
	paths := map[string]bool{}
	for _, e := range errs {
		paths[e.Path.String()] = true
	}
	for _, p := range []string{"a", "b", "c"} {
		if !paths[p] {
			t.Errorf("missing error for path %s", p)
		}
	}
}

// TestConvertNewObjectDeepCopy covers that the result is a new object
// (nested maps and slices are copied, not shared with the input).
func TestConvertNewObjectDeepCopy(t *testing.T) {
	input := map[string]any{
		"nested": map[string]any{"x": "1"},
		"arr":    []any{"a", "b"},
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("nested").Key("x"), KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)

	// The nested map in the result should be a different map than the input.
	resultNested := resultMap["nested"].(map[string]any)
	inputNested := input["nested"].(map[string]any)
	if reflect.ValueOf(resultNested).Pointer() == reflect.ValueOf(inputNested).Pointer() {
		t.Error("nested map is shared with input; expected a new map")
	}

	// The undeclared "arr" slice should also be copied.
	resultArr := resultMap["arr"].([]any)
	inputArr := input["arr"].([]any)
	if reflect.ValueOf(resultArr).Pointer() == reflect.ValueOf(inputArr).Pointer() {
		t.Error("slice is shared with input; expected a new slice")
	}
}

// TestConvertFloat covers strict float conversion.
func TestConvertFloat(t *testing.T) {
	input := map[string]any{
		"f": "1.5",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("f"), KindFloat64)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["f"] != 1.5 {
		t.Errorf("expected 1.5, got %v", resultMap["f"])
	}
}

// TestConvertFloatOverflow covers float32 overflow.
func TestConvertFloatOverflow(t *testing.T) {
	input := map[string]any{
		"f": "1e40",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("f"), KindFloat32)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result on float32 overflow, got %v", result)
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errOverflow) {
		t.Fatalf("expected overflow error, got %v", errs)
	}
}

// TestConvertString covers string conversion (loose behavior preserved).
func TestConvertString(t *testing.T) {
	input := map[string]any{
		"s": 123,
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("s"), KindString)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["s"] != "123" {
		t.Errorf("expected \"123\", got %v", resultMap["s"])
	}
}

// TestConvertUint covers unsigned integer conversion.
func TestConvertUint(t *testing.T) {
	input := map[string]any{
		"u": "42",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("u"), KindUint8)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["u"] != uint8(42) {
		t.Errorf("expected uint8(42), got %v (%T)", resultMap["u"], resultMap["u"])
	}
}

// TestConvertUintNegative covers unsigned rejection of negative values.
func TestConvertUintNegative(t *testing.T) {
	input := map[string]any{
		"u": "-1",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("u"), KindUint8)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result on negative unsigned, got %v", result)
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errOverflow) {
		t.Fatalf("expected overflow error for negative unsigned, got %v", errs)
	}
}

// TestConvertEmptySchema covers an empty schema (deep copy, no conversions).
func TestConvertEmptySchema(t *testing.T) {
	input := map[string]any{
		"a": "1",
		"b": []any{"x"},
	}

	schema := NewSchema()

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["a"] != "1" {
		t.Errorf("expected \"1\", got %v", resultMap["a"])
	}
}

// TestConvertPathString covers the Path.String() display format.
func TestConvertPathString(t *testing.T) {
	p := Path{}.Key("items").Index(0).Key("n")
	want := "items[0].n"
	if p.String() != want {
		t.Errorf("expected %q, got %q", want, p.String())
	}
}

// TestConvertBoolStringVariants covers strict bool acceptance of
// "true"/"false" and rejection of other strings.
func TestConvertBoolStringVariants(t *testing.T) {
	tests := []struct {
		input string
		want  bool
		err   bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"True", true, false},
		{"FALSE", false, false},
		{"1", false, true},
		{"0", false, true},
		{"t", false, true},
		{"f", false, true},
		{"yes", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			input := map[string]any{"b": tt.input}
			schema := NewSchema()
			schema.Set(Path{}.Key("b"), KindBool)

			result, errs := Convert(input, schema)
			if tt.err {
				if result != nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				if len(errs) != 1 || !errors.Is(errs[0].Err, errAmbiguousBool) {
					t.Fatalf("expected ambiguous bool error for %q, got %v", tt.input, errs)
				}
			} else {
				if len(errs) != 0 {
					t.Fatalf("unexpected error for %q: %v", tt.input, errs)
				}
				resultMap := result.(map[string]any)
				if resultMap["b"] != tt.want {
					t.Errorf("expected %v, got %v", tt.want, resultMap["b"])
				}
			}
		})
	}
}

// TestConvertFloatWholeNumber covers strict float conversion of whole
// numbers from strings (e.g. "12" -> 12.0).
func TestConvertFloatWholeNumber(t *testing.T) {
	input := map[string]any{
		"f": "12",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("f"), KindFloat64)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["f"] != float64(12) {
		t.Errorf("expected 12.0, got %v", resultMap["f"])
	}
}

// TestConvertIntegerFromFloat covers strict integer conversion of a float
// value (whole number accepted, fractional rejected).
func TestConvertIntegerFromFloat(t *testing.T) {
	// Whole number float -> accepted.
	input := map[string]any{"n": float64(12)}
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	resultMap := result.(map[string]any)
	if resultMap["n"] != 12 {
		t.Errorf("expected 12, got %v", resultMap["n"])
	}

	// Fractional float -> rejected.
	input2 := map[string]any{"n": float64(12.5)}
	result2, errs2 := Convert(input2, schema)
	if result2 != nil {
		t.Fatalf("expected nil result for fractional float")
	}
	if len(errs2) != 1 || !errors.Is(errs2[0].Err, errLossy) {
		t.Fatalf("expected lossy error, got %v", errs2)
	}
}

// TestConvertIntegerFromStringDecimal covers strict integer parsing of
// strings with decimal points.
func TestConvertIntegerFromStringDecimal(t *testing.T) {
	// "12.0" is a whole number -> accepted.
	input := map[string]any{"n": "12.0"}
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	resultMap := result.(map[string]any)
	if resultMap["n"] != 12 {
		t.Errorf("expected 12, got %v", resultMap["n"])
	}

	// "12.5" is fractional -> rejected.
	input2 := map[string]any{"n": "12.5"}
	result2, errs2 := Convert(input2, schema)
	if result2 != nil {
		t.Fatalf("expected nil result for \"12.5\"")
	}
	if len(errs2) != 1 || !errors.Is(errs2[0].Err, errLossy) {
		t.Fatalf("expected lossy error, got %v", errs2)
	}
}

// TestConvertNestedMapAndSlice covers recursive conversion of nested
// maps and slices.
func TestConvertNestedMapAndSlice(t *testing.T) {
	input := map[string]any{
		"a": map[string]any{
			"b": []any{
				map[string]any{"c": "1"},
				map[string]any{"c": "2"},
			},
		},
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("a").Key("b").Index(0).Key("c"), KindInt)
	schema.Set(Path{}.Key("a").Key("b").Index(1).Key("c"), KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	a := resultMap["a"].(map[string]any)
	b := a["b"].([]any)
	c0 := b[0].(map[string]any)
	c1 := b[1].(map[string]any)

	if c0["c"] != 1 || c1["c"] != 2 {
		t.Errorf("expected c=1 and c=2, got %v and %v", c0["c"], c1["c"])
	}
}

// TestConvertErrorPathAccuracy covers that error paths accurately reflect
// the location of the failure.
func TestConvertErrorPathAccuracy(t *testing.T) {
	input := map[string]any{
		"items": []any{
			map[string]any{"n": "1"},
			map[string]any{"n": "bad"},
			map[string]any{"n": "3"},
		},
	}

	schema := NewSchema()
	for i := 0; i < 3; i++ {
		schema.Set(Path{}.Key("items").Index(i).Key("n"), KindInt)
	}

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	wantPath := "items[1].n"
	if errs[0].Path.String() != wantPath {
		t.Errorf("expected path %q, got %q", wantPath, errs[0].Path.String())
	}
}

// TestConvertBoolFromBool covers strict bool conversion of actual bool
// values.
func TestConvertBoolFromBool(t *testing.T) {
	input := map[string]any{"b": true}
	schema := NewSchema()
	schema.Set(Path{}.Key("b"), KindBool)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	resultMap := result.(map[string]any)
	if resultMap["b"] != true {
		t.Errorf("expected true, got %v", resultMap["b"])
	}
}

// TestConvertInt8Range covers the full int8 range.
func TestConvertInt8Range(t *testing.T) {
	tests := []struct {
		input string
		want  int8
		err   bool
	}{
		{"-128", -128, false},
		{"127", 127, false},
		{"128", 0, true},  // overflow
		{"-129", 0, true}, // overflow
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			input := map[string]any{"n": tt.input}
			schema := NewSchema()
			schema.Set(Path{}.Key("n"), KindInt8)

			result, errs := Convert(input, schema)
			if tt.err {
				if result != nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				if len(errs) != 1 || !errors.Is(errs[0].Err, errOverflow) {
					t.Fatalf("expected overflow error for %q, got %v", tt.input, errs)
				}
			} else {
				if len(errs) != 0 {
					t.Fatalf("unexpected error for %q: %v", tt.input, errs)
				}
				resultMap := result.(map[string]any)
				if resultMap["n"] != tt.want {
					t.Errorf("expected %d, got %v", tt.want, resultMap["n"])
				}
			}
		})
	}
}

// TestConvertResultNotPublishedOnError covers that no result is published
// when any path fails, even if other paths succeed.
func TestConvertResultNotPublishedOnError(t *testing.T) {
	input := map[string]any{
		"good": "1",
		"bad":  "1.5",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("good"), KindInt)
	schema.Set(Path{}.Key("bad"), KindInt)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result when any path fails, got %v", result)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

// TestConvertLooseFunctionsUnchanged covers that the existing loose ToX
// functions still behave as before (regression check).
func TestConvertLooseFunctionsUnchanged(t *testing.T) {
	// These are the loose behaviors that must not change.
	if ToInt("1.5") != 1 {
		t.Errorf("ToInt(\"1.5\") = %d, expected 1", ToInt("1.5"))
	}
	if ToBool("1") != true {
		t.Errorf("ToBool(\"1\") = %v, expected true", ToBool("1"))
	}
	if ToInt8(999) != -25 {
		t.Errorf("ToInt8(999) = %d, expected -25", ToInt8(999))
	}
	if ToInt(nil) != 0 {
		t.Errorf("ToInt(nil) = %d, expected 0", ToInt(nil))
	}
}

// TestConvertFloat32Precision covers float32 precision handling.
func TestConvertFloat32Precision(t *testing.T) {
	input := map[string]any{"f": "3.14"}
	schema := NewSchema()
	schema.Set(Path{}.Key("f"), KindFloat32)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	resultMap := result.(map[string]any)
	f := resultMap["f"].(float32)
	if math.Abs(float64(f)-3.14) > 0.01 {
		t.Errorf("expected ~3.14, got %v", f)
	}
}

// TestConvertMultipleErrorsInSamePath covers that multiple errors in the
// same path are all reported (complete error list).
func TestConvertMultipleErrorsInSamePath(t *testing.T) {
	input := map[string]any{
		"a": "1.5",
		"b": "2.5",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("a"), KindInt)
	schema.Set(Path{}.Key("b"), KindInt)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

// TestConvertPathWithDotsInKey covers that map keys containing dots are
// handled correctly (no string dot guessing).
func TestConvertPathWithDotsInKey(t *testing.T) {
	input := map[string]any{
		"a.b": "1",
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("a.b"), KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	resultMap := result.(map[string]any)
	if resultMap["a.b"] != 1 {
		t.Errorf("expected 1, got %v", resultMap["a.b"])
	}
}

// TestConvertErrorListIsComplete covers the requirement that the error
// list is complete and not truncated.
func TestConvertErrorListIsComplete(t *testing.T) {
	input := map[string]any{
		"a": "1.5",
		"b": "2.5",
		"c": "3.5",
	}

	schema := NewSchema()
	for _, k := range []string{"a", "b", "c"} {
		schema.Set(Path{}.Key(k), KindInt)
	}

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (complete list), got %d: %v", len(errs), errs)
	}
}

// TestConvertPointerInput covers conversion of pointer input values.
func TestConvertPointerInput(t *testing.T) {
	n := "12"
	input := map[string]any{
		"n": &n,
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["n"] != 12 {
		t.Errorf("expected 12, got %v", resultMap["n"])
	}
}

// TestConvertRootLevelScalar covers conversion of a scalar at the root
// (empty path).
func TestConvertRootLevelScalar(t *testing.T) {
	input := "12"

	schema := NewSchema()
	schema.Set(Path{}, KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if result != 12 {
		t.Errorf("expected 12, got %v", result)
	}
}

// TestConvertRootLevelScalarError covers a lossy conversion at the root.
func TestConvertRootLevelScalarError(t *testing.T) {
	input := "1.5"

	schema := NewSchema()
	schema.Set(Path{}, KindInt)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errLossy) {
		t.Fatalf("expected lossy error, got %v", errs)
	}
	if errs[0].Path.String() != "" {
		t.Errorf("expected empty path, got %q", errs[0].Path.String())
	}
}

// TestConvertJSONNumber covers conversion of json.Number values.
func TestConvertJSONNumber(t *testing.T) {
	input := map[string]any{
		"n": json.Number("12"),
	}

	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	resultMap := result.(map[string]any)
	if resultMap["n"] != 12 {
		t.Errorf("expected 12, got %v", resultMap["n"])
	}
}

// TestConvertNilInput covers conversion of nil input.
func TestConvertNilInput(t *testing.T) {
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt)

	result, errs := Convert(nil, schema)
	if result != nil {
		t.Fatalf("expected nil result for nil input, got %v", result)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

// TestConvertNilSchema covers conversion with a nil schema.
func TestConvertNilSchema(t *testing.T) {
	input := map[string]any{"a": "1"}

	result, errs := Convert(input, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestConvertBoolFromNumber covers strict bool rejection of numbers.
func TestConvertBoolFromNumber(t *testing.T) {
	input := map[string]any{"b": 1}
	schema := NewSchema()
	schema.Set(Path{}.Key("b"), KindBool)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errAmbiguousBool) {
		t.Fatalf("expected ambiguous bool error, got %v", errs)
	}
}

// TestConvertStringFromNumber covers string conversion of numbers
// (loose behavior preserved).
func TestConvertStringFromNumber(t *testing.T) {
	input := map[string]any{"s": 42}
	schema := NewSchema()
	schema.Set(Path{}.Key("s"), KindString)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	resultMap := result.(map[string]any)
	if resultMap["s"] != "42" {
		t.Errorf("expected \"42\", got %v", resultMap["s"])
	}
}

// TestConvertInt64 covers int64 conversion.
func TestConvertInt64(t *testing.T) {
	input := map[string]any{"n": "9223372036854775807"}
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt64)

	result, errs := Convert(input, schema)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	resultMap := result.(map[string]any)
	if resultMap["n"] != int64(9223372036854775807) {
		t.Errorf("expected max int64, got %v", resultMap["n"])
	}
}

// TestConvertInt64Overflow covers int64 overflow.
func TestConvertInt64Overflow(t *testing.T) {
	input := map[string]any{"n": "9223372036854775808"}
	schema := NewSchema()
	schema.Set(Path{}.Key("n"), KindInt64)

	result, errs := Convert(input, schema)
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if len(errs) != 1 || !errors.Is(errs[0].Err, errOverflow) {
		t.Fatalf("expected overflow error, got %v", errs)
	}
}
