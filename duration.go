// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sync"
	"time"
)

// maxDurationTextLength is the maximum length of a text input that will be
// parsed as a duration number. Longer inputs are rejected before parsing to
// avoid spending work on pathological strings.
const maxDurationTextLength = 1024

// durationUnits stores the unit declared for named numeric types.
var durationUnits sync.Map // map[reflect.Type]time.Duration

// RegisterDurationUnit declares that values of type T are expressed in the
// given unit. T must be a numeric kind.
//
// When a value of type T is converted with ToDurationInUnitE, the registered
// unit is used instead of the unit passed to the conversion function. This is
// how named numeric types carry their own unit meaning.
//
// Registering a non-numeric type panics.
func RegisterDurationUnit[T any](unit time.Duration) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	if t == nil {
		panic("cast: RegisterDurationUnit: cannot register nil type")
	}

	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
	default:
		panic(fmt.Sprintf("cast: RegisterDurationUnit: type %s is not numeric", t))
	}

	durationUnits.Store(t, unit)
}

// lookupDurationUnit returns the unit registered for the type of i, if any.
func lookupDurationUnit(i any) (time.Duration, bool) {
	if i == nil {
		return 0, false
	}

	t := reflect.TypeOf(i)
	if u, ok := durationUnits.Load(t); ok {
		return u.(time.Duration), true
	}

	return 0, false
}

// ToDurationInUnitE converts a numeric value to a [time.Duration], interpreting
// the input as a multiple of the given unit.
//
// The conversion is exact: the result must be an integral number of
// nanoseconds that fits in an int64. Fractional nanoseconds and values that
// overflow int64 are rejected with an error rather than truncated. Non-finite
// floats (NaN and Inf) are rejected. Floats are interpreted by their exact
// binary value, not a rounded decimal approximation.
//
// If i is already a [time.Duration], it is returned unchanged — it is never
// scaled again. Named numeric types use their registered unit (see
// RegisterDurationUnit) if one is registered.
//
// Text inputs are length-limited before parsing. The Stringer interface is
// never called to guess the input.
func ToDurationInUnitE(i any, unit time.Duration) (time.Duration, error) {
	i, _ = indirect(i)

	// A time.Duration is already a duration. Never scale it again.
	if d, ok := i.(time.Duration); ok {
		return d, nil
	}

	// Named numeric types may carry their own declared unit.
	if u, ok := lookupDurationUnit(i); ok {
		unit = u
	}

	switch v := i.(type) {
	case nil:
		return 0, nil
	case string:
		return durationFromString(v, unit)
	case json.Number:
		return durationFromString(string(v), unit)
	case float64:
		return durationFromFloat64(v, unit)
	case float32:
		return durationFromFloat64(float64(v), unit)
	case int:
		return durationFromRat(big.NewRat(int64(v), 1), unit)
	case int8:
		return durationFromRat(big.NewRat(int64(v), 1), unit)
	case int16:
		return durationFromRat(big.NewRat(int64(v), 1), unit)
	case int32:
		return durationFromRat(big.NewRat(int64(v), 1), unit)
	case int64:
		return durationFromRat(big.NewRat(v, 1), unit)
	case uint:
		return durationFromRat(new(big.Rat).SetInt(new(big.Int).SetUint64(uint64(v))), unit)
	case uint8:
		return durationFromRat(new(big.Rat).SetInt(new(big.Int).SetUint64(uint64(v))), unit)
	case uint16:
		return durationFromRat(new(big.Rat).SetInt(new(big.Int).SetUint64(uint64(v))), unit)
	case uint32:
		return durationFromRat(new(big.Rat).SetInt(new(big.Int).SetUint64(uint64(v))), unit)
	case uint64:
		return durationFromRat(new(big.Rat).SetInt(new(big.Int).SetUint64(v)), unit)
	case float64EProvider:
		f, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf(errorMsgWith, i, i, time.Duration(0), err)
		}

		return durationFromFloat64(f, unit)
	case float64Provider:
		return durationFromFloat64(v.Float64(), unit)
	default:
		if i, ok := resolveAlias(i); ok {
			return ToDurationInUnitE(i, unit)
		}

		return 0, fmt.Errorf(errorMsg, i, i, time.Duration(0))
	}
}

// ToDurationInUnit converts a numeric value to a [time.Duration], interpreting
// the input as a multiple of the given unit.
func ToDurationInUnit(i any, unit time.Duration) time.Duration {
	d, _ := ToDurationInUnitE(i, unit)

	return d
}

// durationFromFloat64 computes f * unit exactly, requiring an integral
// nanosecond result within int64.
func durationFromFloat64(f float64, unit time.Duration) (time.Duration, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("unable to cast non-finite float %v to time.Duration", f)
	}

	r := new(big.Rat).SetFloat64(f)
	if r == nil {
		// SetFloat64 returns nil only for non-finite floats, which we already
		// rejected above; this is defensive.
		return 0, fmt.Errorf("unable to cast float %v to time.Duration", f)
	}

	return durationFromRat(r, unit)
}

// durationFromString parses s as an exact number and multiplies by unit.
// The string is length-limited before parsing.
func durationFromString(s string, unit time.Duration) (time.Duration, error) {
	if len(s) > maxDurationTextLength {
		return 0, fmt.Errorf("duration text too long: %d bytes > %d limit", len(s), maxDurationTextLength)
	}

	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("unable to cast %q to time.Duration: not a valid number", s)
	}

	return durationFromRat(r, unit)
}

// durationFromRat computes r * unit exactly, requiring an integral nanosecond
// result within int64.
func durationFromRat(r *big.Rat, unit time.Duration) (time.Duration, error) {
	ns := ratMul(r, unit)
	if !ns.IsInt() {
		return 0, fmt.Errorf("unable to cast %v * %s to time.Duration: result is not an integral number of nanoseconds", r, unit)
	}

	n := ns.Num()
	if !n.IsInt64() {
		return 0, fmt.Errorf("unable to cast %v * %s to time.Duration: result overflows int64 nanoseconds", r, unit)
	}

	return time.Duration(n.Int64()), nil
}

// ratMul returns r * unit as an exact rational.
func ratMul(r *big.Rat, unit time.Duration) *big.Rat {
	return new(big.Rat).Mul(r, big.NewRat(int64(unit), 1))
}
