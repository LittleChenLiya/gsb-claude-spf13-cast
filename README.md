# cast

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/spf13/cast/ci.yaml?style=flat-square)](https://github.com/spf13/cast/actions/workflows/ci.yaml)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/mod/github.com/spf13/cast)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/spf13/cast?style=flat-square&color=61CFDD)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/spf13/cast/badge?style=flat-square)](https://deps.dev/go/github.com%252Fspf13%252Fcast)

Easy and safe casting from one type to another in Go

Don’t Panic! ... Cast

## What is Cast?

Cast is a library to convert between different go types in a consistent and easy way.

Cast provides simple functions to easily convert a number to a string, an
interface into a bool, etc. Cast does this intelligently when an obvious
conversion is possible. It doesn’t make any attempts to guess what you meant,
for example you can only convert a string to an int when it is a string
representation of an int such as “8”. Cast was developed for use in
[Hugo](https://gohugo.io), a website engine which uses YAML, TOML or JSON
for meta data.

## Why use Cast?

When working with dynamic data in Go you often need to cast or convert the data
from one type into another. Cast goes beyond just using type assertion (though
it uses that when possible) to provide a very straightforward and convenient
library.

If you are working with interfaces to handle things like dynamic content
you’ll need an easy way to convert an interface into a given type. This
is the library for you.

If you are taking in data from YAML, TOML or JSON or other formats which lack
full types, then Cast is the library for you.

## Usage

Cast provides a handful of To_____ methods. These methods will always return
the desired type. **If input is provided that will not convert to that type, the
0 or nil value for that type will be returned**.

Cast also provides identical methods To_____E. These return the same result as
the To_____ methods, plus an additional error which tells you if it successfully
converted. Using these methods you can tell the difference between when the
input matched the zero value or when the conversion failed and the zero value
was returned.

The following examples are merely a sample of what is available. Please review
the code for a complete set.

### Example ‘ToString’:

    cast.ToString("mayonegg")         // "mayonegg"
    cast.ToString(8)                  // "8"
    cast.ToString(8.31)               // "8.31"
    cast.ToString([]byte("one time")) // "one time"
    cast.ToString(nil)                // ""

	var foo interface{} = "one more time"
    cast.ToString(foo)                // "one more time"


### Example ‘ToInt’:

    cast.ToInt(8)                  // 8
    cast.ToInt(8.31)               // 8
    cast.ToInt("8")                // 8
    cast.ToInt(true)               // 1
    cast.ToInt(false)              // 0

	var eight interface{} = 8
    cast.ToInt(eight)              // 8
    cast.ToInt(nil)                // 0

### Explicit Duration Conversion with Units

`ToDurationInUnitE` converts a numeric value to a `time.Duration` while
declaring the input unit explicitly. Unlike `ToDurationE`, which guesses the
unit from the input (e.g. `"5ms"`), `ToDurationInUnitE` treats the input as a
multiple of the declared unit:

    cast.ToDurationInUnit(0.5, time.Millisecond) // 500000
    cast.ToDurationInUnit(5, time.Second)        // 5000000000
    cast.ToDurationInUnit("5", time.Millisecond) // 5000000

The conversion is exact. The result must be an integral number of nanoseconds
that fits in an `int64`. Fractional nanoseconds and values that overflow
`int64` are rejected with an error rather than truncated. Non-finite floats
(`NaN` and `Inf`) are rejected. Floats are interpreted by their exact binary
value, not a rounded decimal approximation.

If the input is already a `time.Duration`, it is returned unchanged — it is
never scaled again.

#### Registering Units for Named Numeric Types

Named numeric types can carry their own unit meaning. Register a type with
`RegisterDurationUnit` and the registered unit is used instead of the unit
passed to the conversion function:

    type Millis int64
    cast.RegisterDurationUnit[Millis](time.Millisecond)

    cast.ToDurationInUnit(Millis(5), time.Second) // 5000000 (5ms, not 5s)

#### Unit Protocol

The unit protocol is: the input value is a number, and the declared unit is a
`time.Duration` representing the unit of that number. The result is
`input * unit`, computed exactly. Text inputs are length-limited before
parsing. The `Stringer` interface is never called to guess the input.

## Development

The project uses [just](https://just.systems/) to run development tasks.

> [!NOTE]
> For an optimal developer experience, it is recommended to install [devenv](https://devenv.sh/) and [direnv](https://direnv.net/docs/installation.html).

Run the test suite:

```shell
just test
```

Run only the duration-related tests locally:

```shell
go test -run 'TestDuration' -v ./...
```

Run the explicit duration conversion tests locally:

```shell
go test -run 'TestDurationInUnit' -v ./...
```

Run linters:

```shell
just lint
```

Some linter violations can automatically be fixed:

```shell
just fmt
```

## License

The project is licensed under the [MIT License](LICENSE).
