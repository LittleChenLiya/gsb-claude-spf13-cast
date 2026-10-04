// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"errors"
	"fmt"
	"testing"

	"github.com/frankban/quicktest"
)

func toIntKey(k string) (int, error) { return ToIntE(k) }
func toIntVal(v string) (int, error) { return ToIntE(v) }

// Keys "01" and "1" both convert to the int 1: the collision must be
// reported and no value conversion may happen at all.
func TestConvertMapEKeyCollision(t *testing.T) {
	c := quicktest.New(t)

	valCalls := 0
	valFn := func(v string) (int, error) {
		valCalls++
		return ToIntE(v)
	}

	res, err := ConvertMapE(
		[]MapEntry[string, string]{
			{Key: "01", Value: "10"},
			{Key: "1", Value: "20"},
		},
		toIntKey,
		valFn,
	)

	c.Assert(res, quicktest.IsNil)
	c.Assert(err, quicktest.Not(quicktest.IsNil))
	c.Assert(valCalls, quicktest.Equals, 0) // values are never converted on collision

	var coll *KeyCollisionError
	c.Assert(errors.As(err, &coll), quicktest.IsTrue)
	c.Assert(coll.Key, quicktest.Equals, 1)
	c.Assert(coll.First, quicktest.Equals, 0)
	c.Assert(coll.Second, quicktest.Equals, 1)
}

// Without collisions the result carries the converted map plus the
// key provenance table.
func TestConvertMapESuccessWithSources(t *testing.T) {
	c := quicktest.New(t)

	entries := []MapEntry[string, string]{
		{Key: "1", Value: "10"},
		{Key: "2", Value: "20"},
	}

	res, err := ConvertMapE(entries, toIntKey, toIntVal)

	c.Assert(err, quicktest.IsNil)
	c.Assert(res.Map, quicktest.DeepEquals, map[int]int{1: 10, 2: 20})
	c.Assert(res.Sources, quicktest.DeepEquals, map[int]string{1: "1", 2: "2"})
	// The input list is left untouched.
	c.Assert(entries, quicktest.DeepEquals, []MapEntry[string, string]{
		{Key: "1", Value: "10"},
		{Key: "2", Value: "20"},
	})
}

// Illegal key types are reported against the original key position,
// all failures are aggregated, and values are never converted.
func TestConvertMapEKeyErrorsAggregated(t *testing.T) {
	c := quicktest.New(t)

	valCalls := 0
	valFn := func(v string) (int, error) {
		valCalls++
		return ToIntE(v)
	}

	res, err := ConvertMapE(
		[]MapEntry[string, string]{
			{Key: "a", Value: "1"},
			{Key: "2", Value: "2"},
			{Key: "b", Value: "3"},
		},
		toIntKey,
		valFn,
	)

	c.Assert(res, quicktest.IsNil)
	c.Assert(err, quicktest.Not(quicktest.IsNil))
	c.Assert(valCalls, quicktest.Equals, 0)

	var keyErrs []*KeyError
	for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
		var ke *KeyError
		c.Assert(errors.As(e, &ke), quicktest.IsTrue)
		keyErrs = append(keyErrs, ke)
	}
	c.Assert(len(keyErrs), quicktest.Equals, 2)
	c.Assert(keyErrs[0].Index, quicktest.Equals, 0)
	c.Assert(keyErrs[0].Key, quicktest.Equals, "a")
	c.Assert(keyErrs[1].Index, quicktest.Equals, 2)
	c.Assert(keyErrs[1].Key, quicktest.Equals, "b")
}

// Value overflows are reported against the original key position and
// no partial result is delivered.
func TestConvertMapEValueOverflow(t *testing.T) {
	c := quicktest.New(t)

	res, err := ConvertMapE(
		[]MapEntry[string, string]{
			{Key: "1", Value: "100"},
			{Key: "2", Value: "9999999999999999999999"}, // overflows int64
		},
		toIntKey,
		toIntVal,
	)

	c.Assert(res, quicktest.IsNil)
	c.Assert(err, quicktest.Not(quicktest.IsNil))

	var ke *KeyError
	c.Assert(errors.As(err, &ke), quicktest.IsTrue)
	c.Assert(ke.Index, quicktest.Equals, 1)
	c.Assert(ke.Key, quicktest.Equals, "2")
}

// Two-key example: keys are converted to int, values to int, and the
// provenance table records where each converted key came from.
func ExampleConvertMapE() {
	res, err := ConvertMapE(
		[]MapEntry[string, string]{
			{Key: "1", Value: "10"},
			{Key: "2", Value: "20"},
		},
		toIntKey,
		toIntVal,
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(res.Map[1], res.Map[2])
	fmt.Println(res.Sources[1], res.Sources[2])

	// Output:
	// 10 20
	// 1 2
}
