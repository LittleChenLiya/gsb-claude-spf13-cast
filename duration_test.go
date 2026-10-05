// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast_test

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/spf13/cast"
)

func TestDurationInUnit(t *testing.T) {
	c := qt.New(t)

	// 0.5 millisecond is exactly 500000 nanoseconds.
	c.Assert(cast.ToDurationInUnit(0.5, time.Millisecond), qt.Equals, 500000*time.Nanosecond)

	// Integer inputs with explicit units.
	c.Assert(cast.ToDurationInUnit(5, time.Nanosecond), qt.Equals, 5*time.Nanosecond)
	c.Assert(cast.ToDurationInUnit(5, time.Microsecond), qt.Equals, 5*time.Microsecond)
	c.Assert(cast.ToDurationInUnit(5, time.Millisecond), qt.Equals, 5*time.Millisecond)
	c.Assert(cast.ToDurationInUnit(5, time.Second), qt.Equals, 5*time.Second)
	c.Assert(cast.ToDurationInUnit(5, time.Minute), qt.Equals, 5*time.Minute)
	c.Assert(cast.ToDurationInUnit(5, time.Hour), qt.Equals, 5*time.Hour)

	// All integer kinds.
	for _, v := range []any{int(5), int8(5), int16(5), int32(5), int64(5), uint(5), uint8(5), uint16(5), uint32(5), uint64(5)} {
		c.Assert(cast.ToDurationInUnit(v, time.Millisecond), qt.Equals, 5*time.Millisecond)
	}

	// Float kinds.
	c.Assert(cast.ToDurationInUnit(float32(0.5), time.Millisecond), qt.Equals, 500000*time.Nanosecond)
	c.Assert(cast.ToDurationInUnit(float64(0.5), time.Millisecond), qt.Equals, 500000*time.Nanosecond)

	// String and json.Number text inputs.
	c.Assert(cast.ToDurationInUnit("5", time.Millisecond), qt.Equals, 5*time.Millisecond)
	c.Assert(cast.ToDurationInUnit("0.5", time.Millisecond), qt.Equals, 500000*time.Nanosecond)
	c.Assert(cast.ToDurationInUnit(json.Number("5"), time.Millisecond), qt.Equals, 5*time.Millisecond)

	// Negative values.
	c.Assert(cast.ToDurationInUnit(-5, time.Millisecond), qt.Equals, -5*time.Millisecond)
	c.Assert(cast.ToDurationInUnit(-0.5, time.Millisecond), qt.Equals, -500000*time.Nanosecond)

	// nil input.
	c.Assert(cast.ToDurationInUnit(nil, time.Millisecond), qt.Equals, time.Duration(0))
}

func TestDurationInUnitFractionalNanoseconds(t *testing.T) {
	c := qt.New(t)

	// 0.5 nanoseconds is not an integral number of nanoseconds.
	_, err := cast.ToDurationInUnitE(0.5, time.Nanosecond)
	c.Assert(err, qt.IsNotNil)

	// 0.1 nanoseconds (binary float) is not an integral number of nanoseconds.
	_, err = cast.ToDurationInUnitE(0.1, time.Nanosecond)
	c.Assert(err, qt.IsNotNil)

	// 0.9 nanoseconds is not an integral number of nanoseconds.
	_, err = cast.ToDurationInUnitE(0.9, time.Nanosecond)
	c.Assert(err, qt.IsNotNil)

	// float32 0.1 nanoseconds is not an integral number of nanoseconds.
	_, err = cast.ToDurationInUnitE(float32(0.1), time.Nanosecond)
	c.Assert(err, qt.IsNotNil)

	// 0.5 microseconds is exactly 500 nanoseconds (integral).
	d, err := cast.ToDurationInUnitE(0.5, time.Microsecond)
	c.Assert(err, qt.IsNil)
	c.Assert(d, qt.Equals, 500*time.Nanosecond)

	// 0.5 milliseconds is exactly 500000 nanoseconds (integral).
	d, err = cast.ToDurationInUnitE(0.5, time.Millisecond)
	c.Assert(err, qt.IsNil)
	c.Assert(d, qt.Equals, 500000*time.Nanosecond)
}

func TestDurationInUnitOverflow(t *testing.T) {
	c := qt.New(t)

	// uint64 value just above int64 max overflows nanoseconds.
	_, err := cast.ToDurationInUnitE(uint64(math.MaxInt64)+1, time.Nanosecond)
	c.Assert(err, qt.IsNotNil)

	// Large float overflows nanoseconds.
	_, err = cast.ToDurationInUnitE(1e20, time.Nanosecond)
	c.Assert(err, qt.IsNotNil)

	// Large int64 * large unit overflows.
	_, err = cast.ToDurationInUnitE(int64(math.MaxInt64), time.Hour)
	c.Assert(err, qt.IsNotNil)

	// MaxInt64 nanoseconds fits exactly.
	d, err := cast.ToDurationInUnitE(int64(math.MaxInt64), time.Nanosecond)
	c.Assert(err, qt.IsNil)
	c.Assert(d, qt.Equals, time.Duration(math.MaxInt64))
}

func TestDurationInUnitNonFinite(t *testing.T) {
	c := qt.New(t)

	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := cast.ToDurationInUnitE(f, time.Millisecond)
		c.Assert(err, qt.IsNotNil, qt.Commentf("expected error for %v", f))
	}

	// float32 non-finite.
	_, err := cast.ToDurationInUnitE(float32(math.NaN()), time.Millisecond)
	c.Assert(err, qt.IsNotNil)
}

func TestDurationInUnitDurationUnchanged(t *testing.T) {
	c := qt.New(t)

	// A time.Duration is returned unchanged, never scaled again.
	d := 5 * time.Second
	c.Assert(cast.ToDurationInUnit(d, time.Millisecond), qt.Equals, d)

	d = 100 * time.Millisecond
	c.Assert(cast.ToDurationInUnit(d, time.Second), qt.Equals, d)

	// Even a pointer to a duration is unchanged.
	c.Assert(cast.ToDurationInUnit(&d, time.Hour), qt.Equals, d)
}

func TestDurationInUnitRegisteredUnit(t *testing.T) {
	c := qt.New(t)

	type Millis int64
	cast.RegisterDurationUnit[Millis](time.Millisecond)

	// The registered unit is used, not the unit passed to the function.
	c.Assert(cast.ToDurationInUnit(Millis(5), time.Second), qt.Equals, 5*time.Millisecond)
	c.Assert(cast.ToDurationInUnit(Millis(5), time.Nanosecond), qt.Equals, 5*time.Millisecond)

	// A non-registered named type falls back to its underlying kind.
	type Seconds int64
	c.Assert(cast.ToDurationInUnit(Seconds(5), time.Second), qt.Equals, 5*time.Second)
}

// stringerMillis is a named numeric type that implements Stringer.
// It is used to verify that Stringer is not called during conversion.
type stringerMillis int64

var stringerMillisCalls int

func (m stringerMillis) String() string {
	stringerMillisCalls++

	return "5ms"
}

func TestDurationInUnitStringerNotCalled(t *testing.T) {
	c := qt.New(t)

	// Stringer on a non-numeric type must not be called.
	type countingStringer struct {
		called *int
	}
	s := countingStringer{called: new(int)}
	_, err := cast.ToDurationInUnitE(s, time.Millisecond)
	c.Assert(err, qt.IsNotNil)
	c.Assert(*s.called, qt.Equals, 0, qt.Commentf("String() was called %d times", *s.called))

	// Stringer on a named numeric type must not be called either.
	stringerMillisCalls = 0
	_, err = cast.ToDurationInUnitE(stringerMillis(5), time.Millisecond)
	c.Assert(err, qt.IsNil)
	c.Assert(stringerMillisCalls, qt.Equals, 0, qt.Commentf("String() was called %d times", stringerMillisCalls))
}

func TestDurationInUnitTextLengthLimited(t *testing.T) {
	c := qt.New(t)

	// A long string is rejected before parsing. The error must be about the
	// length, not about parsing, proving the length check happens first.
	long := strings.Repeat("1", 2000) + "ms"
	_, err := cast.ToDurationInUnitE(long, time.Millisecond)
	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "too long")

	// A string just over the limit is rejected.
	over := strings.Repeat("1", 1025)
	_, err = cast.ToDurationInUnitE(over, time.Millisecond)
	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "too long")

	// A string at the limit is parsed (and fails on its content).
	at := strings.Repeat("1", 1024)
	_, err = cast.ToDurationInUnitE(at, time.Millisecond)
	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Not(qt.Contains), "too long")
}

func TestDurationInUnitInvalidText(t *testing.T) {
	c := qt.New(t)

	// Duration strings with units are not numbers and must fail.
	for _, s := range []string{"5ms", "5s", "5h", "abc", "1.2.3"} {
		_, err := cast.ToDurationInUnitE(s, time.Millisecond)
		c.Assert(err, qt.IsNotNil, qt.Commentf("expected error for %q", s))
	}
}

func TestDurationInUnitFloatExactBinary(t *testing.T) {
	c := qt.New(t)

	// 0.5 is exactly representable in binary, so 0.5 ms = 500000 ns exactly.
	c.Assert(cast.ToDurationInUnit(0.5, time.Millisecond), qt.Equals, 500000*time.Nanosecond)

	// 0.25 is exactly representable, so 0.25 ms = 250000 ns exactly.
	c.Assert(cast.ToDurationInUnit(0.25, time.Millisecond), qt.Equals, 250000*time.Nanosecond)

	// 0.1 is not exactly representable. 0.1 ms is slightly more than 100000 ns,
	// so it is not an integral number of nanoseconds and must fail.
	_, err := cast.ToDurationInUnitE(0.1, time.Millisecond)
	c.Assert(err, qt.IsNotNil)
}

func ExampleToDurationInUnit() {
	// 0.5 millisecond is exactly 500000 nanoseconds.
	d, err := cast.ToDurationInUnitE(0.5, time.Millisecond)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d\n", d)

	// A time.Duration is returned unchanged, never scaled again.
	d, err = cast.ToDurationInUnitE(5*time.Second, time.Millisecond)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d\n", d)

	// Fractional nanoseconds are rejected.
	_, err = cast.ToDurationInUnitE(0.5, time.Nanosecond)
	fmt.Println(err != nil)

	// Output:
	// 500000
	// 5000000000
	// true
}
