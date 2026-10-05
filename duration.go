// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"sync"
	"time"
)

// Unit declares the unit a numeric input to [ToDurationInE] is expressed in.
//
// The unit is part of the conversion contract: [ToDurationInE] never guesses
// the meaning of a bare number. The caller either passes the unit explicitly
// or registers it for a named numeric type with [RegisterDurationUnit].
type Unit struct {
	name  string
	nanos int64 // nanoseconds per one unit, always > 0
}

// String returns the short name of the unit (e.g. "ms").
func (u Unit) String() string { return u.name }

// NanosPerUnit reports how many nanoseconds a single unit represents.
func (u Unit) NanosPerUnit() int64 { return u.nanos }

// Predefined units for explicit duration conversion.
var (
	Nanosecond  = Unit{"ns", int64(time.Nanosecond)}
	Microsecond = Unit{"µs", int64(time.Microsecond)}
	Millisecond = Unit{"ms", int64(time.Millisecond)}
	Second      = Unit{"s", int64(time.Second)}
	Minute      = Unit{"min", int64(time.Minute)}
	Hour        = Unit{"h", int64(time.Hour)}
)

// maxDurationStringLen bounds the length of textual input accepted by
// [ToDurationInE]. The length is checked before any parsing happens.
const maxDurationStringLen = 256

// durationNumber matches the numeric types a unit may be registered for.
type durationNumber interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// durationTypeUnits maps a named numeric type to its registered [Unit].
var durationTypeUnits sync.Map // reflect.Type -> Unit

// RegisterDurationUnit registers the unit a named numeric type is expressed
// in (e.g. type Milliseconds int -> [Millisecond]).
//
// Once registered, values of that type are converted by [ToDurationInE]
// using the registered unit, regardless of the unit passed by the caller:
// the declared meaning of the type wins over the call site.
//
// Registering [time.Duration] panics: a duration already carries its unit
// and is never rescaled. Registrations for unnamed (built-in) types are
// accepted but ignored, as built-ins always use the unit passed by the
// caller.
func RegisterDurationUnit[T durationNumber](unit Unit) {
	var zero T
	t := reflect.TypeOf(zero)
	if t == reflect.TypeOf(time.Duration(0)) {
		panic("cast: time.Duration already carries its unit and is never rescaled")
	}

	durationTypeUnits.Store(t, unit)
}

// ToDurationIn casts any value to a [time.Duration], interpreting numeric
// inputs in the given unit. It is the non-error returning variant of
// [ToDurationInE].
func ToDurationIn(i any, unit Unit) time.Duration {
	v, _ := ToDurationInE(i, unit)
	return v
}

// ToDurationInE casts any value to a [time.Duration] using explicit,
// precise conversion rules:
//
//   - Numeric inputs are interpreted in the declared unit. The result in
//     nanoseconds is computed with exact arithmetic: it must be a whole
//     number of nanoseconds and fit in an int64, otherwise an error is
//     returned. No float truncation ever takes place.
//   - Floating point inputs are interpreted by their exact binary value
//     (e.g. 0.5 is exactly 1/2, while 0.1 is not exactly 1/10). Non-finite
//     values (NaN, +/-Inf) are rejected.
//   - A [time.Duration] input is returned unchanged; it is never rescaled
//     by the declared unit.
//   - Named numeric types must have their unit registered with
//     [RegisterDurationUnit]; unregistered named types are rejected instead
//     of guessing (in particular, a fmt.Stringer is never consulted).
//   - Strings are length-limited (see maxDurationStringLen) before parsing:
//     first as a Go duration string ("1h30m"), then as a plain number in
//     the declared unit.
//
// The plain [ToDuration] / [ToDurationE] functions are unaffected by these
// rules and keep their historical behavior.
func ToDurationInE(i any, unit Unit) (time.Duration, error) {
	if unit.nanos <= 0 {
		return 0, fmt.Errorf("cast: invalid duration unit %q", unit.name)
	}

	i, _ = indirect(i)

	switch v := i.(type) {
	case time.Duration:
		// A duration already carries its unit: never rescale it.
		return v, nil
	case int:
		return scaleInt64ToDuration(int64(v), unit)
	case int8:
		return scaleInt64ToDuration(int64(v), unit)
	case int16:
		return scaleInt64ToDuration(int64(v), unit)
	case int32:
		return scaleInt64ToDuration(int64(v), unit)
	case int64:
		return scaleInt64ToDuration(v, unit)
	case uint:
		return scaleUint64ToDuration(uint64(v), unit)
	case uint8:
		return scaleUint64ToDuration(uint64(v), unit)
	case uint16:
		return scaleUint64ToDuration(uint64(v), unit)
	case uint32:
		return scaleUint64ToDuration(uint64(v), unit)
	case uint64:
		return scaleUint64ToDuration(v, unit)
	case float32:
		return scaleFloat64ToDuration(float64(v), unit)
	case float64:
		return scaleFloat64ToDuration(v, unit)
	case string:
		return stringToDurationIn(v, unit)
	case nil:
		return 0, nil
	default:
		return namedToDurationIn(i)
	}
}

// namedToDurationIn converts a named numeric type using its registered
// unit. It never falls back to reflection guesses or fmt.Stringer.
func namedToDurationIn(i any) (time.Duration, error) {
	t := reflect.TypeOf(i)
	if t == nil {
		return 0, nil
	}

	reg, ok := durationTypeUnits.Load(t)
	if !ok {
		return 0, fmt.Errorf("cast: cannot convert type %T to time.Duration: no unit registered for it (see RegisterDurationUnit)", i)
	}

	unit := reg.(Unit)
	v := reflect.ValueOf(i)

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return scaleInt64ToDuration(v.Int(), unit)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return scaleUint64ToDuration(v.Uint(), unit)
	case reflect.Float32, reflect.Float64:
		return scaleFloat64ToDuration(v.Float(), unit)
	default:
		return 0, fmt.Errorf("cast: cannot convert type %T to time.Duration: registered unit on non-numeric kind %s", i, v.Kind())
	}
}

// scaleInt64ToDuration multiplies v by the nanoseconds of the unit, failing
// on int64 overflow instead of wrapping.
func scaleInt64ToDuration(v int64, unit Unit) (time.Duration, error) {
	n := unit.nanos
	if v != 0 && (v > math.MaxInt64/n || v < math.MinInt64/n) {
		return 0, fmt.Errorf("cast: %d %s overflows time.Duration (int64 nanoseconds)", v, unit.name)
	}

	return time.Duration(v * n), nil
}

// scaleUint64ToDuration multiplies v by the nanoseconds of the unit,
// failing if the result does not fit in an int64.
func scaleUint64ToDuration(v uint64, unit Unit) (time.Duration, error) {
	n := uint64(unit.nanos)
	if v != 0 && v > uint64(math.MaxInt64)/n {
		return 0, fmt.Errorf("cast: %d %s overflows time.Duration (int64 nanoseconds)", v, unit.name)
	}

	return time.Duration(v * n), nil
}

// scaleFloat64ToDuration converts f units to a duration using exact
// arithmetic on the real binary value of the float. Non-finite inputs,
// results with a fractional nanosecond, and results outside the int64
// range are rejected; the float is never truncated toward zero.
func scaleFloat64ToDuration(f float64, unit Unit) (time.Duration, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("cast: non-finite float %v cannot be converted to time.Duration", f)
	}

	// SetFloat64 captures the exact binary value of f as a rational.
	r := new(big.Rat).SetFloat64(f)
	r.Mul(r, big.NewRat(unit.nanos, 1))

	if !r.IsInt() {
		return 0, fmt.Errorf("cast: %v %s does not resolve to a whole number of nanoseconds", f, unit.name)
	}

	n := r.Num()
	if !n.IsInt64() {
		return 0, fmt.Errorf("cast: %v %s overflows time.Duration (int64 nanoseconds)", f, unit.name)
	}

	return time.Duration(n.Int64()), nil
}

// stringToDurationIn parses textual input. The length limit is enforced
// before any parsing is attempted.
func stringToDurationIn(s string, unit Unit) (time.Duration, error) {
	if len(s) > maxDurationStringLen {
		return 0, fmt.Errorf("cast: duration string of %d bytes exceeds the %d byte limit", len(s), maxDurationStringLen)
	}

	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// A plain number is interpreted in the declared unit, using the same
	// exact arithmetic as numeric inputs.
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return scaleInt64ToDuration(v, unit)
	}

	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return scaleUint64ToDuration(v, unit)
	}

	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return scaleFloat64ToDuration(f, unit)
	}

	return 0, errors.New("cast: cannot parse string as a time.Duration")
}
