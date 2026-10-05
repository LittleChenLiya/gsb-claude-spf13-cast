// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/spf13/cast"
)

// Milliseconds is a named numeric type with a registered unit.
type Milliseconds int

// Seconds64 is a named float type with a registered unit.
type Seconds64 float64

// unregisteredMillis is a named numeric type without a registered unit.
type unregisteredMillis int

func init() {
	cast.RegisterDurationUnit[Milliseconds](cast.Millisecond)
	cast.RegisterDurationUnit[Seconds64](cast.Second)
}

// countingStringer records how often String is called.
type countingStringer struct {
	calls *int
}

func (s countingStringer) String() string {
	*s.calls++
	return "5s"
}

// countingStringerNumber is a named numeric type that is also a Stringer.
type countingStringerNumber int

var countingStringerNumberCalls int

func (n countingStringerNumber) String() string {
	countingStringerNumberCalls++
	return fmt.Sprintf("%dns", int(n))
}

func TestToDurationInExact(t *testing.T) {
	c := qt.New(t)

	for _, test := range []struct {
		input  any
		unit   cast.Unit
		expect time.Duration
	}{
		// The headline case: 0.5 milliseconds is exactly 500000 nanoseconds.
		{0.5, cast.Millisecond, 500000 * time.Nanosecond},
		{float32(0.5), cast.Millisecond, 500000 * time.Nanosecond},
		{0.25, cast.Second, 250 * time.Millisecond},
		{2, cast.Millisecond, 2 * time.Millisecond},
		{int64(-3), cast.Second, -3 * time.Second},
		{uint64(7), cast.Nanosecond, 7 * time.Nanosecond},
		{uint32(2), cast.Hour, 2 * time.Hour},
		{1.5, cast.Microsecond, 1500 * time.Nanosecond},
		{0, cast.Hour, 0},
		{nil, cast.Second, 0},
		// Strings: duration syntax and plain numbers in the declared unit.
		{"1h30m", cast.Nanosecond, 90 * time.Minute},
		{"2", cast.Millisecond, 2 * time.Millisecond},
		{"0.5", cast.Millisecond, 500000 * time.Nanosecond},
		// An existing duration is never rescaled.
		{5 * time.Second, cast.Millisecond, 5 * time.Second},
		{time.Duration(-7), cast.Hour, time.Duration(-7)},
		// Named types use their registered unit, not the caller's.
		{Milliseconds(2), cast.Nanosecond, 2 * time.Millisecond},
		{Seconds64(0.5), cast.Nanosecond, 500 * time.Millisecond},
	} {
		got, err := cast.ToDurationInE(test.input, test.unit)
		c.Assert(err, qt.IsNil, qt.Commentf("input %#v", test.input))
		c.Assert(got, qt.Equals, test.expect, qt.Commentf("input %#v", test.input))

		c.Assert(cast.ToDurationIn(test.input, test.unit), qt.Equals, test.expect)
	}
}

func TestToDurationInFractionalNanosecond(t *testing.T) {
	c := qt.New(t)

	// 1.5 nanoseconds cannot be represented: no truncation to 1.
	_, err := cast.ToDurationInE(1.5, cast.Nanosecond)
	c.Assert(err, qt.ErrorMatches, `cast: 1\.5 ns does not resolve to a whole number of nanoseconds`)

	// 0.1 is not exactly 1/10 in binary floating point, so 0.1 ms is not a
	// whole number of nanoseconds under exact interpretation.
	_, err = cast.ToDurationInE(0.1, cast.Millisecond)
	c.Assert(err, qt.ErrorMatches, `cast: 0\.1 ms does not resolve to a whole number of nanoseconds`)

	// The same applies to textual floats.
	_, err = cast.ToDurationInE("1.5", cast.Nanosecond)
	c.Assert(err, qt.ErrorMatches, `cast: 1\.5 ns does not resolve to a whole number of nanoseconds`)
}

func TestToDurationInOverflow(t *testing.T) {
	c := qt.New(t)

	// uint64 inputs beyond the int64 range must fail, not wrap.
	_, err := cast.ToDurationInE(uint64(math.MaxUint64), cast.Nanosecond)
	c.Assert(err, qt.ErrorMatches, `cast: \d+ ns overflows time\.Duration \(int64 nanoseconds\)`)

	_, err = cast.ToDurationInE(uint64(math.MaxInt64)/2+1, cast.Second)
	c.Assert(err, qt.ErrorMatches, `cast: \d+ s overflows time\.Duration \(int64 nanoseconds\)`)

	// int64 inputs that overflow once scaled must fail as well.
	_, err = cast.ToDurationInE(int64(math.MaxInt64), cast.Hour)
	c.Assert(err, qt.ErrorMatches, `cast: -?\d+ h overflows time\.Duration \(int64 nanoseconds\)`)

	_, err = cast.ToDurationInE(int64(math.MinInt64), cast.Hour)
	c.Assert(err, qt.ErrorMatches, `cast: -?\d+ h overflows time\.Duration \(int64 nanoseconds\)`)

	// Floats beyond the int64 range fail instead of being truncated.
	_, err = cast.ToDurationInE(1e30, cast.Second)
	c.Assert(err, qt.ErrorMatches, `cast: 1e\+30 s overflows time\.Duration \(int64 nanoseconds\)`)
}

func TestToDurationInNonFinite(t *testing.T) {
	c := qt.New(t)

	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := cast.ToDurationInE(f, cast.Second)
		c.Assert(err, qt.ErrorMatches, `cast: non-finite float .* cannot be converted to time\.Duration`)
	}
}

func TestToDurationInNeverCallsStringer(t *testing.T) {
	c := qt.New(t)

	// A Stringer is never consulted to guess the input, not even on the
	// error path.
	calls := 0
	_, err := cast.ToDurationInE(countingStringer{calls: &calls}, cast.Second)
	c.Assert(err, qt.ErrorMatches, `cast: cannot convert type cast_test\.countingStringer to time\.Duration: no unit registered for it \(see RegisterDurationUnit\)`)
	c.Assert(calls, qt.Equals, 0)

	// A named numeric type with a String method is rejected while
	// unregistered, again without calling String.
	countingStringerNumberCalls = 0
	_, err = cast.ToDurationInE(countingStringerNumber(5), cast.Second)
	c.Assert(err, qt.ErrorMatches, `cast: cannot convert type cast_test\.countingStringerNumber to time\.Duration: no unit registered for it \(see RegisterDurationUnit\)`)
	c.Assert(countingStringerNumberCalls, qt.Equals, 0)

	// An unregistered named numeric type is rejected instead of guessed.
	_, err = cast.ToDurationInE(unregisteredMillis(5), cast.Second)
	c.Assert(err, qt.ErrorMatches, `cast: cannot convert type cast_test\.unregisteredMillis to time\.Duration: no unit registered for it \(see RegisterDurationUnit\)`)
}

func TestToDurationInStringLengthLimit(t *testing.T) {
	c := qt.New(t)

	// The length limit is enforced before parsing.
	tooLong := strings.Repeat("1", 257) + "ns"
	_, err := cast.ToDurationInE(tooLong, cast.Nanosecond)
	c.Assert(err, qt.ErrorMatches, `cast: duration string of 259 bytes exceeds the 256 byte limit`)

	// A string at the limit is still parsed.
	atLimit := strings.Repeat("9", 256)
	_, err = cast.ToDurationInE(atLimit, cast.Nanosecond)
	c.Assert(err, qt.ErrorMatches, `cast: .* ns overflows time\.Duration \(int64 nanoseconds\)`)

	// Garbage within the limit fails to parse.
	_, err = cast.ToDurationInE("not a duration", cast.Second)
	c.Assert(err, qt.ErrorMatches, `cast: cannot parse string as a time\.Duration`)
}

func TestToDurationInPointer(t *testing.T) {
	c := qt.New(t)

	v := 0.5
	got, err := cast.ToDurationInE(&v, cast.Millisecond)
	c.Assert(err, qt.IsNil)
	c.Assert(got, qt.Equals, 500000*time.Nanosecond)

	d := 5 * time.Second
	got, err = cast.ToDurationInE(&d, cast.Nanosecond)
	c.Assert(err, qt.IsNil)
	c.Assert(got, qt.Equals, 5*time.Second)
}
