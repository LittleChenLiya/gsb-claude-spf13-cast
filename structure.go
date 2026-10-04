// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Scalar is the type parameter constraint for scalar targets accepted by
// [StructConverter]: strings, booleans and all [Number] types.
type Scalar interface {
	string | bool | Number
}

// ScalarKind identifies a supported scalar destination type without using
// generics at the registration site.
type ScalarKind int

// Supported scalar destination kinds.
const (
	KindString ScalarKind = iota + 1
	KindBool
	KindInt
	KindInt8
	KindInt16
	KindInt32
	KindInt64
	KindUint
	KindUint8
	KindUint16
	KindUint32
	KindUint64
	KindFloat32
	KindFloat64
)

// String returns the lowercase Go name of the kind.
func (k ScalarKind) String() string {
	switch k {
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindInt8:
		return "int8"
	case KindInt16:
		return "int16"
	case KindInt32:
		return "int32"
	case KindInt64:
		return "int64"
	case KindUint:
		return "uint"
	case KindUint8:
		return "uint8"
	case KindUint16:
		return "uint16"
	case KindUint32:
		return "uint32"
	case KindUint64:
		return "uint64"
	case KindFloat32:
		return "float32"
	case KindFloat64:
		return "float64"
	default:
		return "unknown"
	}
}

func scalarKindOf[T Scalar]() ScalarKind {
	var t T

	switch any(t).(type) {
	case string:
		return KindString
	case bool:
		return KindBool
	case int:
		return KindInt
	case int8:
		return KindInt8
	case int16:
		return KindInt16
	case int32:
		return KindInt32
	case int64:
		return KindInt64
	case uint:
		return KindUint
	case uint8:
		return KindUint8
	case uint16:
		return KindUint16
	case uint32:
		return KindUint32
	case uint64:
		return KindUint64
	case float32:
		return KindFloat32
	case float64:
		return KindFloat64
	default:
		panic("cast: unsupported scalar type")
	}
}

const (
	minInt  = -1 << (strconv.IntSize - 1)
	maxInt  = 1<<(strconv.IntSize-1) - 1
	maxUint = 1<<strconv.IntSize - 1
)

// PathSegment identifies a single element in a [Path].
//
// Exactly one of Key and Index is meaningful: a Key segment selects an entry
// of a map, while an Index segment selects an element of a slice. Keys are
// matched verbatim; unlike dotted shorthand notations, segments never split
// or reinterpret key contents.
type PathSegment struct {
	// Key selects a map entry. It is ignored when IsIndex is true.
	Key string

	// Index selects a slice element. It is only used when IsIndex is true
	// and must be non-negative (negative indices never match anything).
	Index int

	// IsIndex reports whether this segment is a slice index rather than a
	// map key.
	IsIndex bool
}

// Key returns a map-key [PathSegment]. A key may contain any character,
// including '.' and '['; such characters are only ever interpreted as data.
func Key(key string) PathSegment {
	return PathSegment{Key: key}
}

// At returns a slice-index [PathSegment].
func At(index int) PathSegment {
	return PathSegment{Index: index, IsIndex: true}
}

// Path is a sequence of [PathSegment] values addressed from the root of the
// converted value. It is explicit by design: it never performs ambiguous
// dot-splitting of map keys.
type Path []PathSegment

// ParsePath parses a textual path into an explicit [Path].
//
// Two notations are accepted, and they may be freely mixed:
//
//   - Dot notation: "a.b.c" selects the map keys "a", "b" and "c" in order.
//     Bare keys contain letters, digits, '_' and '-' and must not start with
//     a digit (write "[0]" to select an array index rather than a key named
//     "0").
//   - Bracket notation: "[key]" selects a map key verbatim; '["a.b"]'
//     (double quotes) and "['a.b']" (single quotes) select the literal key
//     "a.b" without dot splitting. Bracket contents that parse as a
//     non-negative base-10 integer without leading zeros select an explicit
//     array index, so "[0]" is an index while "[01]", "[-1]" and '["0"]' are
//     keys.
//
// The empty string denotes the root path. ParsePath returns an error for
// malformed bracket expressions, out-of-range indices or stray characters,
// rather than guessing what the caller meant.
func ParsePath(s string) (Path, error) {
	p := Path{}

	i := 0
	leading := true

	for i < len(s) {
		switch s[i] {
		case '.':
			if leading || i == len(s)-1 {
				return nil, fmt.Errorf("cast: invalid path %q: empty key component", s)
			}

			i++
			leading = true

			continue
		case '[':
			seg, next, err := parseBracket(s, i)
			if err != nil {
				return nil, err
			}

			p = append(p, seg)
			i = next
			leading = false

			continue
		}

		if !leading {
			return nil, fmt.Errorf("cast: invalid path %q: unexpected character %q", s, string(s[i]))
		}

		if s[i] >= '0' && s[i] <= '9' {
			return nil, fmt.Errorf("cast: invalid path %q: use [%c] for array indices, not a bare numeric component", s, s[i])
		}

		start := i

		for i < len(s) && isBareKeyByte(s[i]) {
			i++
		}

		if start == i {
			return nil, fmt.Errorf("cast: invalid path %q: unexpected character %q", s, string(s[i]))
		}

		p = append(p, Key(s[start:i]))
		leading = false
	}

	return p, nil
}

func isBareKeyByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '_' || b == '-':
		return true
	}

	return false
}

func parseBracket(s string, open int) (PathSegment, int, error) {
	rel := strings.IndexByte(s[open:], ']')
	if rel < 0 {
		return PathSegment{}, 0, fmt.Errorf("cast: invalid path %q: unterminated '['", s)
	}

	closeIdx := open + rel
	body := strings.TrimSpace(s[open+1 : closeIdx])
	next := closeIdx + 1

	// Quoted body: always a literal map key.
	if len(body) >= 2 && (body[0] == '"' || body[0] == '\'') {
		quote := body[0]
		if body[len(body)-1] != quote {
			return PathSegment{}, 0, fmt.Errorf("cast: invalid path %q: unterminated quoted key", s)
		}

		key, err := unquoteKey(body)
		if err != nil {
			return PathSegment{}, 0, fmt.Errorf("cast: invalid path %q: %w", s, err)
		}

		return Key(key), next, nil
	}

	// Unquoted non-negative integer without leading zeros: an explicit index.
	if body != "" && isDecimalIndex(body) {
		idx, err := strconv.Atoi(body)
		if err != nil {
			return PathSegment{}, 0, fmt.Errorf("cast: invalid path %q: index out of range", s)
		}

		return At(idx), next, nil
	}

	if body == "" {
		return PathSegment{}, 0, fmt.Errorf("cast: invalid path %q: empty bracket component", s)
	}

	return Key(body), next, nil
}

func isDecimalIndex(body string) bool {
	if body[0] == '0' && len(body) > 1 {
		return false
	}

	for i := 0; i < len(body); i++ {
		if body[i] < '0' || body[i] > '9' {
			return false
		}
	}

	return true
}

func unquoteKey(q string) (string, error) {
	if q[0] == '"' {
		v, err := strconv.Unquote(q)
		if err != nil {
			return "", err
		}

		return v, nil
	}

	// Single-quoted keys: support the common backslash escapes only.
	inner := q[1 : len(q)-1]

	var b strings.Builder

	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c != '\\' || i == len(inner)-1 {
			b.WriteByte(c)

			continue
		}

		i++

		switch inner[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		case '\'':
			b.WriteByte('\'')
		default:
			b.WriteByte(inner[i])
		}
	}

	return b.String(), nil
}

// String renders the path back in the notation accepted by [ParsePath].
func (p Path) String() string {
	var b strings.Builder

	for _, seg := range p {
		if seg.IsIndex {
			fmt.Fprintf(&b, "[%d]", seg.Index)

			continue
		}

		if isBareKey(seg.Key) {
			if b.Len() > 0 {
				b.WriteByte('.')
			}

			b.WriteString(seg.Key)
		} else {
			b.WriteByte('[')
			b.WriteString(strconv.Quote(seg.Key))
			b.WriteByte(']')
		}
	}

	return b.String()
}

func isBareKey(s string) bool {
	if s == "" {
		return false
	}

	if s[0] >= '0' && s[0] <= '9' {
		return false
	}

	for i := 0; i < len(s); i++ {
		if !isBareKeyByte(s[i]) {
			return false
		}
	}

	return true
}

// PathError describes a conversion failure at a specific [Path].
type PathError struct {
	// Path is the explicit path of the failing value, relative to the
	// converted root (the empty path denotes the root value itself).
	Path Path

	// Err is the underlying conversion error.
	Err error
}

func (e *PathError) Error() string {
	return fmt.Sprintf("cast: strict conversion failed at %q: %v", e.Path, e.Err)
}

// Unwrap exposes the underlying error for use with [errors.Is] and
// [errors.As].
func (e *PathError) Unwrap() error {
	return e.Err
}

// ConvertFunc is a custom conversion rule registered for an explicit path.
// It receives the value found at that path and returns the replacement value.
// When it returns a non-nil error, that error is reported at the path.
type ConvertFunc func(value any) (any, error)

// StructConverter recursively converts nested maps and slices according to
// scalar targets declared via explicit paths.
//
// It is independent from the permissive ToX functions: conversions run in
// strict mode, which rejects lossy integer conversions, ambiguous booleans
// and out-of-range values. nil values and missing keys are preserved and are
// never reported as errors. A converted value is only published when every
// declared path converts successfully; otherwise Convert returns nil together
// with the complete list of [PathError] values.
//
// The input itself is never modified: [StructConverter.Convert] builds and
// returns a new value.
type StructConverter struct {
	targets map[string]scalarTarget
	custom  map[string]ConvertFunc
}

type scalarTarget struct {
	path Path
	kind ScalarKind
}

// NewStructConverter creates an empty [StructConverter].
func NewStructConverter() *StructConverter {
	return &StructConverter{
		targets: map[string]scalarTarget{},
		custom:  map[string]ConvertFunc{},
	}
}

// Target declares that every value at path must be strictly converted to the
// scalar type T. Later calls for the same path replace the earlier target.
//
// T must be one of string, bool or a [Number] type.
func Target[T Scalar](c *StructConverter, path Path) *StructConverter {
	return c.Target(path, scalarKindOf[T]())
}

// Target declares that every value at path must be strictly converted to kind.
// Later calls for the same path replace the earlier target.
func (c *StructConverter) Target(path Path, kind ScalarKind) *StructConverter {
	cp := clonePath(path)
	c.targets[pathKey(cp)] = scalarTarget{path: cp, kind: kind}

	return c
}

// Custom registers an explicit conversion rule for path. A registered rule
// takes precedence over any scalar target at the same path; it is the only
// mechanism through which custom conversions run, and it is looked up by
// exact path match, never by dotted string guessing.
//
// Later calls for the same path replace the earlier rule.
func (c *StructConverter) Custom(path Path, fn ConvertFunc) *StructConverter {
	cp := clonePath(path)
	c.custom[pathKey(cp)] = fn

	return c
}

func clonePath(p Path) Path {
	cp := make(Path, len(p))
	copy(cp, p)

	return cp
}

func pathKey(p Path) string {
	var b strings.Builder

	for _, seg := range p {
		if seg.IsIndex {
			b.WriteByte('i')
			fmt.Fprintf(&b, "%d", seg.Index)
			b.WriteByte(';')
		} else {
			b.WriteByte('k')
			b.WriteString(strconv.Itoa(len(seg.Key)))
			b.WriteByte(':')
			b.WriteString(seg.Key)
		}
	}

	return b.String()
}

// Convert recursively converts v according to the declared targets and
// custom rules and returns the new value.
//
// Conversion is all-or-nothing: if one or more declared paths fail, the
// returned value is nil and errs reports every failure (each a *[PathError]).
// On success errs is nil. The original v is never mutated.
func (c *StructConverter) Convert(v any) (any, []error) {
	w := &convertWalker{
		targets: c.targets,
		custom:  c.custom,
	}

	out := w.convert(v, nil)
	if len(w.errs) > 0 {
		return nil, w.errs
	}

	return out, nil
}

// ConvertE is like [StructConverter.Convert] but, on failure, joins all path
// errors into a single error.
func (c *StructConverter) ConvertE(v any) (any, error) {
	out, errs := c.Convert(v)
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return out, nil
}

type convertWalker struct {
	targets map[string]scalarTarget
	custom  map[string]ConvertFunc
	errs    []error
}

func (w *convertWalker) convert(v any, path Path) any {
	if fn, ok := w.custom[pathKey(path)]; ok {
		nv, err := fn(v)
		if err != nil {
			w.errs = append(w.errs, &PathError{Path: clonePath(path), Err: err})

			return nil
		}

		return nv
	}

	// nil (whether an element value or a map entry value) is preserved and
	// is not an error. Missing keys are a separate concept: they are simply
	// never visited and therefore remain absent from the result.
	if v == nil {
		return nil
	}

	if target, ok := w.targets[pathKey(path)]; ok {
		nv, err := strictConvert(target.kind, v)
		if err != nil {
			w.errs = append(w.errs, &PathError{Path: clonePath(path), Err: err})

			return nil
		}

		return nv
	}

	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = w.convert(item, appendPath(path, Key(k)))
		}

		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = w.convert(item, appendPath(path, At(i)))
		}

		return out
	default:
		// Untargeted, non-container values are copied through unchanged.
		return v
	}
}

func appendPath(p Path, seg PathSegment) Path {
	cp := make(Path, 0, len(p)+1)
	cp = append(cp, p...)
	cp = append(cp, seg)

	return cp
}

var strictIntegerRe = regexp.MustCompile(`^[-+]?[0-9]+$`)

// strictConvert converts v to kind without losing information.
//
// It deliberately differs from the permissive ToX functions:
//   - integers require an exact integral value (no decimal truncation,
//     no bool-to-int interpretation and no exponent syntax);
//   - booleans only accept actual bools and the strings "true"/"false";
//   - values outside the destination range are rejected rather than
//     saturated or wrapped;
//   - nil is handled by the caller (preserved), and empty strings are not
//     coerced to zero.
func strictConvert(kind ScalarKind, v any) (any, error) {
	switch kind {
	case KindString:
		switch s := v.(type) {
		case string:
			return s, nil
		case []byte:
			return string(s), nil
		}

		return nil, fmt.Errorf("strict conversion to string: %v of type %T is not a string", v, v)
	case KindBool:
		switch b := v.(type) {
		case bool:
			return b, nil
		case string:
			if b != "true" && b != "false" {
				return nil, fmt.Errorf("strict conversion to bool: ambiguous string %q (only \"true\" and \"false\" are accepted)", b)
			}

			return b == "true", nil
		}

		return nil, fmt.Errorf("strict conversion to bool: %v of type %T is not a bool", v, v)
	case KindInt, KindInt8, KindInt16, KindInt32, KindInt64:
		return strictInt(kind, v)
	case KindUint, KindUint8, KindUint16, KindUint32, KindUint64:
		return strictUint(kind, v)
	case KindFloat32, KindFloat64:
		return strictFloat(kind, v)
	default:
		return nil, fmt.Errorf("strict conversion: unsupported target kind %v", kind)
	}
}

func strictInt(kind ScalarKind, v any) (any, error) {
	iv, exact, err := strictSignedValue(kind, v)
	if err != nil {
		return nil, err
	}

	if !exact {
		return nil, fmt.Errorf("strict conversion to %s: %q is not an exact integer", kind, fmt.Sprint(v))
	}

	min, max := signedRange(kind)
	if iv < min || iv > max {
		return nil, fmt.Errorf("strict conversion to %s: %d is out of range [%d, %d]", kind, iv, min, max)
	}

	switch kind {
	case KindInt:
		return int(iv), nil
	case KindInt8:
		return int8(iv), nil
	case KindInt16:
		return int16(iv), nil
	case KindInt32:
		return int32(iv), nil
	default:
		return iv, nil
	}
}

func strictUint(kind ScalarKind, v any) (any, error) {
	var uv uint64

	switch n := v.(type) {
	case uint:
		uv = uint64(n)
	case uint8:
		uv = uint64(n)
	case uint16:
		uv = uint64(n)
	case uint32:
		uv = uint64(n)
	case uint64:
		uv = n
	case int, int8, int16, int32, int64:
		iv := signedOf(n)
		if iv < 0 {
			return nil, fmt.Errorf("strict conversion to %s: %d is negative", kind, iv)
		}

		uv = uint64(iv)
	case float32, float64:
		f := float64Of(n)
		if !isExactUint(f) {
			return nil, fmt.Errorf("strict conversion to %s: %v is not an exact non-negative integer", kind, f)
		}

		uv = uint64(f)
	case string:
		s := n
		if s == "" || s[0] == '-' || !strictIntegerRe.MatchString(s) {
			return nil, fmt.Errorf("strict conversion to %s: %q is not an exact non-negative integer", kind, s)
		}

		x, err := strconv.ParseUint(strings.TrimPrefix(s, "+"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("strict conversion to %s: %w", kind, err)
		}

		uv = x
	case json.Number:
		s := n.String()
		if !strictIntegerRe.MatchString(s) || s[0] == '-' {
			return nil, fmt.Errorf("strict conversion to %s: %q is not an exact non-negative integer", kind, s)
		}

		x, err := strconv.ParseUint(strings.TrimPrefix(s, "+"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("strict conversion to %s: %w", kind, err)
		}

		uv = x
	default:
		return nil, fmt.Errorf("strict conversion to %s: %v of type %T is not a number", kind, v, v)
	}

	umax := unsignedMax(kind)
	if uv > umax {
		return nil, fmt.Errorf("strict conversion to %s: %d is out of range [0, %d]", kind, uv, umax)
	}

	switch kind {
	case KindUint:
		return uint(uv), nil
	case KindUint8:
		return uint8(uv), nil
	case KindUint16:
		return uint16(uv), nil
	case KindUint32:
		return uint32(uv), nil
	default:
		return uv, nil
	}
}

func isExactUint(f float64) bool {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}

	if f < 0 || f > float64(math.MaxUint64) {
		return false
	}

	return f == math.Trunc(f)
}

func strictSignedValue(kind ScalarKind, v any) (int64, bool, error) {
	switch n := v.(type) {
	case int:
		return int64(n), true, nil
	case int8:
		return int64(n), true, nil
	case int16:
		return int64(n), true, nil
	case int32:
		return int64(n), true, nil
	case int64:
		return n, true, nil
	case uint:
		return int64(n), true, nil
	case uint8:
		return int64(n), true, nil
	case uint16:
		return int64(n), true, nil
	case uint32:
		return int64(n), true, nil
	case uint64:
		return int64(n), true, nil
	case float32, float64:
		f := float64Of(n)

		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false, fmt.Errorf("strict conversion to %s: %v is not finite", kind, f)
		}

		if f != math.Trunc(f) {
			return 0, false, nil
		}

		if f < float64(math.MinInt64) || f > float64(math.MaxInt64) {
			return 0, false, fmt.Errorf("strict conversion to %s: %v is out of int64 range", kind, f)
		}

		return int64(f), true, nil
	case string:
		s := n
		if !strictIntegerRe.MatchString(s) {
			return 0, false, nil
		}

		iv, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("strict conversion to %s: %w", kind, err)
		}

		return iv, true, nil
	case json.Number:
		s := n.String()
		if !strictIntegerRe.MatchString(s) {
			return 0, false, nil
		}

		iv, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("strict conversion to %s: %w", kind, err)
		}

		return iv, true, nil
	default:
		return 0, false, fmt.Errorf("strict conversion to %s: %v of type %T is not a number", kind, v, v)
	}
}

func strictFloat(kind ScalarKind, v any) (any, error) {
	var f float64

	switch n := v.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	case int:
		f = float64(n)
	case int8:
		f = float64(n)
	case int16:
		f = float64(n)
	case int32:
		f = float64(n)
	case int64:
		f = float64(n)
	case uint:
		f = float64(n)
	case uint8:
		f = float64(n)
	case uint16:
		f = float64(n)
	case uint32:
		f = float64(n)
	case uint64:
		f = float64(n)
	case string:
		x, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return nil, fmt.Errorf("strict conversion to %s: %w", kind, err)
		}

		f = x
	case json.Number:
		x, err := n.Float64()
		if err != nil {
			return nil, fmt.Errorf("strict conversion to %s: %w", kind, err)
		}

		f = x
	default:
		return nil, fmt.Errorf("strict conversion to %s: %v of type %T is not a number", kind, v, v)
	}

	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("strict conversion to %s: %v is not finite", kind, f)
	}

	if kind == KindFloat32 {
		// Round-trip through float32 to detect values that do not fit.
		if math.IsInf(float64(float32(f)), 0) {
			return nil, fmt.Errorf("strict conversion to float32: %v is out of range", f)
		}

		return float32(f), nil
	}

	return f, nil
}

func signedOf(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	}

	return 0
}

func float64Of(v any) float64 {
	switch n := v.(type) {
	case float32:
		return float64(n)
	case float64:
		return n
	}

	return 0
}

func signedRange(kind ScalarKind) (int64, int64) {
	switch kind {
	case KindInt:
		return minInt, maxInt
	case KindInt8:
		return math.MinInt8, math.MaxInt8
	case KindInt16:
		return math.MinInt16, math.MaxInt16
	case KindInt32:
		return math.MinInt32, math.MaxInt32
	default:
		return math.MinInt64, math.MaxInt64
	}
}

func unsignedMax(kind ScalarKind) uint64 {
	switch kind {
	case KindUint:
		return maxUint
	case KindUint8:
		return math.MaxUint8
	case KindUint16:
		return math.MaxUint16
	case KindUint32:
		return math.MaxUint32
	default:
		return math.MaxUint64
	}
}
