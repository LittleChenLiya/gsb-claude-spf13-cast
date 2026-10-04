// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cast"
)

// TestConvertMapEKeyCollision verifies the acceptance requirement: keys "01"
// and "1" collide when converted to int (target type equality), no value is
// converted, and the error reports both original key positions.
//
// Targeted command:
//
//	go test -run 'TestConvertMapEKeyCollision' -v .
func TestConvertMapEKeyCollision(t *testing.T) {
	entries := []cast.Entry[any, any]{
		{Key: "01", Value: "first"},
		{Key: "1", Value: "second"},
	}

	valueConversions := 0
	valFn := func(v any) (string, error) {
		valueConversions++
		return cast.ToStringE(v)
	}

	m, sources, err := cast.ConvertMapE(entries, cast.ToIntE, valFn)
	if err == nil {
		t.Fatal("expected a collision error, got nil")
	}
	if m != nil {
		t.Fatalf("expected nil result map on error, got %v", m)
	}
	if sources != nil {
		t.Fatalf("expected nil sources on error, got %v", sources)
	}
	if valueConversions != 0 {
		t.Fatalf("expected 0 value conversions on collision, got %d", valueConversions)
	}

	// The aggregated error must identify both original keys and positions.
	msg := err.Error()
	if !strings.Contains(msg, `"01"`) {
		t.Fatalf("error should mention original key \"01\", got: %s", msg)
	}
	if !strings.Contains(msg, `"1"`) {
		t.Fatalf("error should mention original key \"1\", got: %s", msg)
	}

	var collErr *cast.KeyCollisionError
	if !errors.As(err, &collErr) {
		t.Fatalf("expected a KeyCollisionError in the error chain, got: %T", err)
	}
	if collErr.Index != 1 || collErr.ExistingIndex != 0 {
		t.Fatalf("unexpected collision positions: %+v", collErr)
	}
}

// TestConvertMapESuccess verifies that a successful conversion delivers the
// result map and a source table tracing each result key back to its origin,
// in input order.
func TestConvertMapESuccess(t *testing.T) {
	entries := []cast.Entry[any, any]{
		{Key: "1", Value: "one"},
		{Key: "2", Value: "two"},
	}

	m, sources, err := cast.ConvertMapE(entries, cast.ToIntE, cast.ToStringE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[int]string{1: "one", 2: "two"}
	if !reflect.DeepEqual(m, expected) {
		t.Fatalf("expected result %v, got %v", expected, m)
	}

	// The source table must be in input order and trace each result key back
	// to its original key.
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %+v", len(sources), sources)
	}
	for i, s := range sources {
		if s.Key != i+1 {
			t.Fatalf("source %d: expected key %d, got %d", i, i+1, s.Key)
		}
	}
	if sources[0].Origin != "1" || sources[1].Origin != "2" {
		t.Fatalf("unexpected source origins: %+v", sources)
	}
}

// TestConvertMapEStringKeysDoNotCollide shows that key comparison follows the
// target type's equality: "01" and "1" are distinct as strings, so no
// collision occurs when the target key type is string.
func TestConvertMapEStringKeysDoNotCollide(t *testing.T) {
	entries := []cast.Entry[any, any]{
		{Key: "01", Value: "a"},
		{Key: "1", Value: "b"},
	}

	m, _, err := cast.ConvertMapE(entries, cast.ToStringE, cast.ToStringE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["01"] != "a" || m["1"] != "b" {
		t.Fatalf("expected both distinct keys in result, got %v", m)
	}
}

// TestConvertMapEKeyFnCalledOncePerKey verifies the key conversion function is
// called exactly once per key.
func TestConvertMapEKeyFnCalledOncePerKey(t *testing.T) {
	entries := []cast.Entry[any, any]{
		{Key: "1", Value: "a"},
		{Key: "2", Value: "b"},
		{Key: "3", Value: "c"},
	}

	calls := 0
	keyFn := func(k any) (int, error) {
		calls++
		return cast.ToIntE(k)
	}

	if _, _, err := cast.ConvertMapE(entries, keyFn, cast.ToStringE); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != len(entries) {
		t.Fatalf("expected keyFn to be called %d times (once per key), got %d", len(entries), calls)
	}
}

// TestConvertMapEAggregatesAllKeyErrors verifies that illegal key types are
// reported collectively with their original key positions, and no result is
// delivered.
func TestConvertMapEAggregatesAllKeyErrors(t *testing.T) {
	entries := []cast.Entry[any, any]{
		{Key: "abc", Value: "a"}, // invalid int
		{Key: "1", Value: "b"},
		{Key: "xyz", Value: "c"}, // invalid int
	}

	m, sources, err := cast.ConvertMapE(entries, cast.ToIntE, cast.ToStringE)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if m != nil || sources != nil {
		t.Fatalf("expected nil result on error, got m=%v sources=%v", m, sources)
	}

	var convErrs cast.ConvertErrors
	if !errors.As(err, &convErrs) {
		t.Fatalf("expected ConvertErrors in chain, got %T", err)
	}
	if len(convErrs) != 2 {
		t.Fatalf("expected 2 aggregated key errors (positions 0 and 2), got %d: %v", len(convErrs), err)
	}

	msg := err.Error()
	if !strings.Contains(msg, `"abc"`) || !strings.Contains(msg, `"xyz"`) {
		t.Fatalf("error should mention both original keys, got: %s", msg)
	}
}

// TestConvertMapEOverflowReported verifies that an overflow is reported with
// its original key position.
func TestConvertMapEOverflowReported(t *testing.T) {
	entries := []cast.Entry[any, any]{
		{Key: "99999999999999999999999", Value: "a"}, // overflows int
	}

	_, _, err := cast.ConvertMapE(entries, cast.ToIntE, cast.ToStringE)
	if err == nil {
		t.Fatal("expected overflow error, got nil")
	}

	var convErr *cast.ConvertError
	if !errors.As(err, &convErr) {
		t.Fatalf("expected a ConvertError in chain, got %T", err)
	}
	if convErr.Index != 0 {
		t.Fatalf("expected error at position 0, got %d", convErr.Index)
	}
}

// TestConvertMapEDoesNotMutateInput verifies the explicit input list is left
// unchanged by the conversion.
func TestConvertMapEDoesNotMutateInput(t *testing.T) {
	entries := []cast.Entry[any, any]{
		{Key: "1", Value: "a"},
		{Key: "2", Value: "b"},
	}

	orig := make([]cast.Entry[any, any], len(entries))
	copy(orig, entries)

	if _, _, err := cast.ConvertMapE(entries, cast.ToIntE, cast.ToStringE); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(entries, orig) {
		t.Fatalf("input list was modified: before=%v after=%v", orig, entries)
	}
}

// TestConvertMapEEmpty verifies an empty input yields an empty result.
func TestConvertMapEEmpty(t *testing.T) {
	m, sources, err := cast.ConvertMapE(nil, cast.ToIntE, cast.ToStringE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 || len(sources) != 0 {
		t.Fatalf("expected empty result, got m=%v sources=%v", m, sources)
	}
}

// ExampleConvertMapE demonstrates converting an explicit, ordered list of
// key/value pairs into a map, with a source table tracing each result key back
// to its origin.
//
// Targeted command:
//
//	go test -run 'ExampleConvertMapE' -v .
func ExampleConvertMapE() {
	entries := []cast.Entry[any, any]{
		{Key: "1", Value: "one"},
		{Key: "2", Value: "two"},
	}

	m, sources, err := cast.ConvertMapE(entries, cast.ToIntE, cast.ToStringE)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(m[1], m[2])
	for _, s := range sources {
		fmt.Printf("%d <- %v\n", s.Key, s.Origin)
	}
	// Output:
	// one two
	// 1 <- 1
	// 2 <- 2
}
