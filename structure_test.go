// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/spf13/cast"
)

// TestStructConverterAcceptance covers the acceptance scenario:
// items[0].n="12", items[1].n="1.5" converted to int must report the exact
// path of the second item, publish no result, and leave the original input
// untouched (the string values must remain strings).
func TestStructConverterAcceptance(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"items": []any{
			map[string]any{"n": "12"},
			map[string]any{"n": "1.5"},
		},
	}

	c := cast.NewStructConverter().
		Target(cast.Path{cast.Key("items"), cast.At(0), cast.Key("n")}, cast.KindInt).
		Target(cast.Path{cast.Key("items"), cast.At(1), cast.Key("n")}, cast.KindInt)

	out, errs := c.Convert(input)
	cqt := qt.New(t)
	cqt.Assert(out, qt.IsNil)
	cqt.Assert(errs, qt.HasLen, 1)

	var pe *cast.PathError
	cqt.Assert(errors.As(errs[0], &pe), qt.IsTrue)
	cqt.Assert(pe.Path.String(), qt.Equals, "items[1].n")
	cqt.Assert(pe.Error(), qt.Contains, "items[1].n")
	cqt.Assert(pe.Error(), qt.Contains, "not an exact integer")

	// The original input must be untouched: both values remain strings.
	items := input["items"].([]any)
	qt.New(t).Assert(items[0].(map[string]any)["n"], qt.Equals, "12")
	qt.New(t).Assert(items[1].(map[string]any)["n"], qt.Equals, "1.5")

	// ConvertE joins all path errors into one.
	_, err := c.ConvertE(input)
	cqt.Assert(err, qt.IsNotNil)
	cqt.Assert(err.Error(), qt.Contains, "items[1].n")
}

// TestStructConverterSuccess verifies that two legal integers produce a new,
// deeply converted object without mutating the input.
func TestStructConverterSuccess(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"items": []any{
			map[string]any{"n": "12"},
			map[string]any{"n": "34"},
		},
		"kept": map[string]any{"s": "unchanged"},
	}

	c := cast.NewStructConverter().
		Target(cast.Path{cast.Key("items"), cast.At(0), cast.Key("n")}, cast.KindInt).
		Target(cast.Path{cast.Key("items"), cast.At(1), cast.Key("n")}, cast.KindInt)

	out, errs := c.Convert(input)
	qt.New(t).Assert(errs, qt.IsNil)

	want := map[string]any{
		"items": []any{
			map[string]any{"n": 12},
			map[string]any{"n": 34},
		},
		"kept": map[string]any{"s": "unchanged"},
	}
	qt.New(t).Assert(out, qt.DeepEquals, want)

	// A new object is returned: mutating it must never affect the input.
	om := out.(map[string]any)
	om["items"].([]any)[0].(map[string]any)["n"] = 999
	delete(om, "kept")

	inItems := input["items"].([]any)
	qt.New(t).Assert(inItems[0].(map[string]any)["n"], qt.Equals, "12")
	qt.New(t).Assert(input["kept"], qt.Not(qt.IsNil))

	// Input strings remain strings.
	items := input["items"].([]any)
	qt.New(t).Assert(items[0].(map[string]any)["n"], qt.Equals, "12")
	qt.New(t).Assert(items[1].(map[string]any)["n"], qt.Equals, "34")
}

// TestStructConverterOverflowNotZero ensures overflow and range errors are
// reported rather than being truncated or wrapped to zero.
func TestStructConverterOverflowNotZero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		path  cast.Path
		value any
		kind  cast.ScalarKind
	}{
		{"int8 overflow", cast.Path{cast.Key("x")}, int64(300), cast.KindInt8},
		{"uint8 overflow", cast.Path{cast.Key("x")}, int64(256), cast.KindUint8},
		{"uint negative", cast.Path{cast.Key("x")}, int64(-1), cast.KindUint},
		{"int64 parse overflow", cast.Path{cast.Key("x")}, "9223372036854775808", cast.KindInt64},
		{"float32 overflow", cast.Path{cast.Key("x")}, "1e40", cast.KindFloat32},
		{"float NaN", cast.Path{cast.Key("x")}, math.NaN(), cast.KindFloat64},
		{"float +Inf", cast.Path{cast.Key("x")}, math.Inf(1), cast.KindFloat32},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := cast.NewStructConverter().Target(tc.path, tc.kind)
			out, errs := c.Convert(map[string]any{"x": tc.value})

			cqt := qt.New(t)
			cqt.Assert(out, qt.IsNil)
			cqt.Assert(errs, qt.HasLen, 1)
			cqt.Assert(errs[0].Error(), qt.Contains, "x")
		})
	}
}

// TestStructConverterAllOrNothing verifies that a failure on one path also
// rejects other paths and that all errors are collected.
func TestStructConverterAllOrNothing(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"a": "1",
		"b": "nope",
		"c": "trueish",
	}

	c := cast.NewStructConverter().
		Target(cast.Path{cast.Key("a")}, cast.KindInt).
		Target(cast.Path{cast.Key("b")}, cast.KindInt).
		Target(cast.Path{cast.Key("c")}, cast.KindBool)

	out, errs := c.Convert(input)
	cqt := qt.New(t)
	cqt.Assert(out, qt.IsNil)
	cqt.Assert(errs, qt.HasLen, 2)

	paths := []string{}
	for _, e := range errs {
		var pe *cast.PathError
		cqt.Assert(errors.As(e, &pe), qt.IsTrue)
		paths = append(paths, pe.Path.String())
	}
	cqt.Assert(paths, qt.ContentEquals, []string{"b", "c"})
}

// TestStructConverterNilAndMissing ensures nil values and missing keys are
// kept separate: nil survives as an explicit null entry, while a key that was
// never present stays absent; neither is an error.
func TestStructConverterNilAndMissing(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"present": nil,
		"number":  "7",
	}

	c := cast.NewStructConverter().
		Target(cast.Path{cast.Key("present")}, cast.KindInt).
		Target(cast.Path{cast.Key("number")}, cast.KindInt).
		// A target at a missing path is simply not visited.
		Target(cast.Path{cast.Key("absent"), cast.Key("n")}, cast.KindInt)

	out, errs := c.Convert(input)
	cqt := qt.New(t)
	cqt.Assert(errs, qt.IsNil)

	om := out.(map[string]any)
	v, ok := om["present"]
	cqt.Assert(ok, qt.IsTrue)
	cqt.Assert(v, qt.IsNil)

	_, ok = om["absent"]
	cqt.Assert(ok, qt.IsFalse)

	cqt.Assert(om["number"], qt.Equals, 7)

	// nil elements inside slices are preserved positionally as well.
	in := []any{"1", nil, "3"}
	c2 := cast.NewStructConverter().
		Target(cast.Path{cast.At(0)}, cast.KindInt).
		Target(cast.Path{cast.At(1)}, cast.KindInt).
		Target(cast.Path{cast.At(2)}, cast.KindInt)

	o2, e2 := c2.Convert(in)
	cqt.Assert(e2, qt.IsNil)
	cqt.Assert(o2, qt.DeepEquals, []any{1, nil, 3})

	// Targets past the end of a slice (explicit indices) are not visited and
	// therefore not errors.
	o3, e3 := cast.NewStructConverter().
		Target(cast.Path{cast.At(5)}, cast.KindInt).
		Convert([]any{"1"})
	cqt.Assert(e3, qt.IsNil)
	cqt.Assert(o3, qt.DeepEquals, []any{"1"})
}

// TestStructConverterCustomRule verifies that custom conversions run only
// from the explicit registry, including nested inputs, and that a custom
// error is reported at the exact registered path.
func TestStructConverterCustomRule(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"items": []any{
			map[string]any{"v": "10"},
			map[string]any{"v": "20"},
		},
	}

	calls := 0
	c := cast.NewStructConverter().
		Custom(cast.Path{cast.Key("items"), cast.At(1), cast.Key("v")}, func(v any) (any, error) {
			calls++

			return strings.ToUpper(v.(string)) + "!", nil
		})

	out, errs := c.Convert(input)
	cqt := qt.New(t)
	cqt.Assert(errs, qt.IsNil)
	cqt.Assert(calls, qt.Equals, 1)

	om := out.(map[string]any)
	items := om["items"].([]any)
	cqt.Assert(items[0].(map[string]any)["v"], qt.Equals, "10")
	cqt.Assert(items[1].(map[string]any)["v"], qt.Equals, "20!")

	// A custom rule failing aborts the publication and keeps the path.
	sentinel := errors.New("boom")
	c2 := cast.NewStructConverter().
		Custom(cast.Path{cast.Key("items"), cast.At(0), cast.Key("v")}, func(any) (any, error) {
			return nil, sentinel
		})

	out2, errs2 := c2.Convert(input)
	cqt.Assert(out2, qt.IsNil)
	cqt.Assert(errs2, qt.HasLen, 1)
	cqt.Assert(errors.Is(errs2[0], sentinel), qt.IsTrue)

	var pe *cast.PathError
	cqt.Assert(errors.As(errs2[0], &pe), qt.IsTrue)
	cqt.Assert(pe.Path.String(), qt.Equals, "items[0].v")

	// Explicit lookup: a dotted custom key does not get dot-split.
	in3 := map[string]any{"a.b": "3"}
	c3 := cast.NewStructConverter().
		Custom(cast.Path{cast.Key("a.b")}, func(v any) (any, error) { return "hit", nil })
	o3, e3 := c3.Convert(in3)
	cqt.Assert(e3, qt.IsNil)
	cqt.Assert(o3.(map[string]any)["a.b"], qt.Equals, "hit")
}

// TestStructConverterStrictVersusLenient documents the behavioral difference
// from the existing permissive ToX functions.
func TestStructConverterStrictVersusLenient(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		value     any
		kind      cast.ScalarKind
		lenientOK bool
		strictOK  bool
	}{
		{"fractional string to int", "1.5", cast.KindInt, true, false},
		{"float truncated to int", float64(1.9), cast.KindInt, true, false},
		{"empty string to int", "", cast.KindInt, true, false},
		{"number to bool", 1, cast.KindBool, true, false},
		{"ambiguous bool string", "t", cast.KindBool, true, false},
		{"nil handled separately", nil, cast.KindInt, true /* ToInt(nil)=0 */, true /* preserved, not converted */},
		{"overflow int8", int64(128), cast.KindInt8, true, false},
		{"bool string", "true", cast.KindBool, true, true},
		{"exact int string", "42", cast.KindInt, true, true},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cqt := qt.New(t)

			// Strict path: via the structural converter.
			conv := cast.NewStructConverter().Target(cast.Path{cast.Key("v")}, tc.kind)

			if tc.value == nil {
				out, errs := conv.Convert(map[string]any{"v": tc.value})
				cqt.Assert(errs, qt.IsNil)
				cqt.Assert(out.(map[string]any)["v"], qt.IsNil)

				return
			}

			_, errs := conv.Convert(map[string]any{"v": tc.value})
			if tc.strictOK {
				cqt.Assert(errs, qt.IsNil)
			} else {
				cqt.Assert(errs, qt.HasLen, 1)
			}

			// Lenient path: via the pre-existing generic ToE.
			_, err := lenientTo(tc.kind, tc.value)
			cqt.Assert(err == nil, qt.Equals, tc.lenientOK,
				qt.Commentf("name=%s value=%v", tc.name, tc.value))
		})
	}

	// Concrete contrast: lenient truncates "1.5" to 1, strict rejects it.
	cqt := qt.New(t)
	loose, err := cast.ToIntE("1.5")
	cqt.Assert(err, qt.IsNil)
	cqt.Assert(loose, qt.Equals, 1)

	_, errs := cast.NewStructConverter().
		Target(cast.Path{cast.Key("n")}, cast.KindInt).
		Convert(map[string]any{"n": "1.5"})
	cqt.Assert(errs, qt.HasLen, 1)

	// Overflow leniently wraps/truncates; strict refuses.
	loose8, err := cast.ToInt8E(int64(300))
	cqt.Assert(err, qt.IsNil)
	cqt.Assert(loose8, qt.Equals, int8(44))

	_, errs = cast.NewStructConverter().
		Target(cast.Path{cast.Key("x")}, cast.KindInt8).
		Convert(map[string]any{"x": int64(300)})
	cqt.Assert(errs, qt.HasLen, 1)
}

func lenientTo(kind cast.ScalarKind, v any) (any, error) {
	switch kind {
	case cast.KindInt:
		return cast.ToIntE(v)
	case cast.KindInt8:
		return cast.ToInt8E(v)
	case cast.KindInt16:
		return cast.ToInt16E(v)
	case cast.KindInt32:
		return cast.ToInt32E(v)
	case cast.KindInt64:
		return cast.ToInt64E(v)
	case cast.KindUint:
		return cast.ToUintE(v)
	case cast.KindUint8:
		return cast.ToUint8E(v)
	case cast.KindUint16:
		return cast.ToUint16E(v)
	case cast.KindUint32:
		return cast.ToUint32E(v)
	case cast.KindUint64:
		return cast.ToUint64E(v)
	case cast.KindFloat32:
		return cast.ToFloat32E(v)
	case cast.KindFloat64:
		return cast.ToFloat64E(v)
	case cast.KindBool:
		return cast.ToBoolE(v)
	case cast.KindString:
		return cast.ToStringE(v)
	default:
		panic("unreachable")
	}
}

// TestGenericTarget exercises the generic registration helper.
func TestGenericTarget(t *testing.T) {
	t.Parallel()

	c := cast.Target[int32](cast.NewStructConverter(), cast.Path{cast.Key("a")})
	out, errs := c.Convert(map[string]any{"a": "5", "b": int64(9)})
	cqt := qt.New(t)
	cqt.Assert(errs, qt.IsNil)
	om := out.(map[string]any)
	cqt.Assert(om["a"], qt.Equals, int32(5))
	cqt.Assert(om["b"], qt.Equals, int64(9))
}

// TestParsePath covers explicit path parsing, including refusal to perform
// ambiguous dotted-string guessing.
func TestParsePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"a", "a"},
		{"a.b.c", "a.b.c"},
		{"items[0].n", "items[0].n"},
		{"[0]", "[0]"},
		{`["a.b"]`, `["a.b"]`},
		{"['a.b']", `["a.b"]`},
		{"a-b_c.x", "a-b_c.x"},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()

			p, err := cast.ParsePath(tc.in)
			cqt := qt.New(t)
			cqt.Assert(err, qt.IsNil)
			cqt.Assert(p.String(), qt.Equals, tc.want)
		})
	}

	bad := []string{
		"a..b", // empty dot component
		".a",   // leading dot
		"a.",   // trailing dot
		"a[b",  // unterminated bracket
		"a[ ]", // empty bracket
		"0a",   // bare component starting with a digit
		`["un`, // unterminated quote
		"a b",  // space outside brackets
	}

	for _, in := range bad {
		in := in

		t.Run("bad/"+in, func(t *testing.T) {
			t.Parallel()

			_, err := cast.ParsePath(in)
			qt.New(t).Assert(err, qt.IsNotNil)
		})
	}

	// Non-index bracket bodies are verbatim keys, never guessed indices.
	for in, want := range map[string]string{
		"a[01]":  "01",
		"a[-1]":  "-1",
		`a["0"]`: "0",
	} {
		in, want := in, want

		t.Run("key/"+in, func(t *testing.T) {
			t.Parallel()

			p, err := cast.ParsePath(in)
			cqt := qt.New(t)
			cqt.Assert(err, qt.IsNil)
			cqt.Assert(p, qt.HasLen, 2)
			cqt.Assert(p[1], qt.DeepEquals, cast.Key(want))
		})
	}

	// Quoted dotted key does not behave like two segments: conversion must
	// look up the literal key "a.b", never nested a -> b.
	p, err := cast.ParsePath(`["a.b"]`)
	qt.New(t).Assert(err, qt.IsNil)
	qt.New(t).Assert(p, qt.DeepEquals, cast.Path{cast.Key("a.b")})

	p2, err := cast.ParsePath("items[1].n")
	qt.New(t).Assert(err, qt.IsNil)
	qt.New(t).Assert(p2, qt.DeepEquals, cast.Path{cast.Key("items"), cast.At(1), cast.Key("n")})
}

// TestPathErrorUnwrap checks error identity through the path wrapper.
func TestPathErrorUnwrap(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("nope")

	c := cast.NewStructConverter().
		Custom(cast.Path{cast.Key("x")}, func(any) (any, error) { return nil, sentinel })

	_, errs := c.Convert(map[string]any{"x": 1})
	cqt := qt.New(t)
	cqt.Assert(errs, qt.HasLen, 1)
	cqt.Assert(errors.Is(errs[0], sentinel), qt.IsTrue)

	var pe *cast.PathError
	cqt.Assert(errors.As(errs[0], &pe), qt.IsTrue)
	cqt.Assert(pe.Path, qt.DeepEquals, cast.Path{cast.Key("x")})
}

// TestStructConverterNestedSlicesAndMaps verifies recursion through deeply
// nested containers and that output shares no container with the input.
func TestStructConverterNestedSlicesAndMaps(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"matrix": []any{
			[]any{map[string]any{"v": "1"}},
			[]any{map[string]any{"v": "2"}},
		},
	}

	c := cast.NewStructConverter().
		Target(cast.Path{cast.Key("matrix"), cast.At(0), cast.At(0), cast.Key("v")}, cast.KindInt).
		Target(cast.Path{cast.Key("matrix"), cast.At(1), cast.At(0), cast.Key("v")}, cast.KindInt)

	out, errs := c.Convert(input)
	qt.New(t).Assert(errs, qt.IsNil)

	want := map[string]any{
		"matrix": []any{
			[]any{map[string]any{"v": 1}},
			[]any{map[string]any{"v": 2}},
		},
	}
	qt.New(t).Assert(out, qt.DeepEquals, want)
	qt.New(t).Assert(input["matrix"].([]any)[0].([]any)[0].(map[string]any)["v"], qt.Equals, "1")
}

// TestStructConverterExactFloats confirms integral floats are accepted by the
// integer targets (no information loss) while fractional ones are rejected.
func TestStructConverterExactFloats(t *testing.T) {
	t.Parallel()

	c := cast.NewStructConverter().Target(cast.Path{cast.Key("v")}, cast.KindInt)

	out, errs := c.Convert(map[string]any{"v": float64(42.0)})
	cqt := qt.New(t)
	cqt.Assert(errs, qt.IsNil)
	cqt.Assert(out.(map[string]any)["v"], qt.Equals, 42)

	_, errs = c.Convert(map[string]any{"v": float64(42.5)})
	cqt.Assert(errs, qt.HasLen, 1)
}
