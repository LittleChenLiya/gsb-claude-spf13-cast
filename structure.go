// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// ScalarKind identifies a target scalar type at a leaf path of a Schema.
type ScalarKind uint8

const (
	// KindString targets a string scalar.
	KindString ScalarKind = iota
	// KindBool targets a bool scalar.
	KindBool
	// KindInt targets an int scalar.
	KindInt
	// KindInt8 targets an int8 scalar.
	KindInt8
	// KindInt16 targets an int16 scalar.
	KindInt16
	// KindInt32 targets an int32 scalar.
	KindInt32
	// KindInt64 targets an int64 scalar.
	KindInt64
	// KindUint targets a uint scalar.
	KindUint
	// KindUint8 targets a uint8 scalar.
	KindUint8
	// KindUint16 targets a uint16 scalar.
	KindUint16
	// KindUint32 targets a uint32 scalar.
	KindUint32
	// KindUint64 targets a uint64 scalar.
	KindUint64
	// KindFloat32 targets a float32 scalar.
	KindFloat32
	// KindFloat64 targets a float64 scalar.
	KindFloat64
)

// Converter converts a scalar value to a target value.
//
// It is the signature used by the explicit custom conversion registry
// maintained by a Schema.
type Converter func(value any) (any, error)

// Path is an explicit sequence of map keys and slice indices that addresses
// a location in a nested structure.
//
// A Path is built from KeyStep and IndexStep values. It is never parsed
// from a dotted string, so there is no ambiguity about whether a segment is
// a map key or a slice index.
type Path []PathStep

// PathStep is a single step in a Path.
//
// Exactly one of Key or Index is meaningful: when IsIndex is false the step
// addresses a map key, and when IsIndex is true it addresses a slice index.
type PathStep struct {
	Key     string
	Index   int
	IsIndex bool
}

// KeyStep returns a PathStep addressing the given map key.
func KeyStep(key string) PathStep {
	return PathStep{Key: key}
}

// IndexStep returns a PathStep addressing the given slice index.
func IndexStep(index int) PathStep {
	return PathStep{Index: index, IsIndex: true}
}

// Key returns a new Path with a map-key step appended.
func (p Path) Key(k string) Path {
	out := make(Path, len(p)+1)
	copy(out, p)
	out[len(p)] = KeyStep(k)
	return out
}

// Index returns a new Path with a slice-index step appended.
func (p Path) Index(i int) Path {
	out := make(Path, len(p)+1)
	copy(out, p)
	out[len(p)] = IndexStep(i)
	return out
}

// String returns an unambiguous display representation of the path.
//
// It is intended for error messages only; paths are never parsed from
// strings.
func (p Path) String() string {
	var b strings.Builder
	for i, s := range p {
		if s.IsIndex {
			fmt.Fprintf(&b, "[%d]", s.Index)
		} else {
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(s.Key)
		}
	}
	return b.String()
}

// ConversionError records a conversion failure at a specific Path.
type ConversionError struct {
	Path Path
	Err  error
}

func (e *ConversionError) Error() string {
	return fmt.Sprintf("at %s: %v", e.Path, e.Err)
}

func (e *ConversionError) Unwrap() error {
	return e.Err
}

// Schema declares target scalar types (and optional custom converters) at
// explicit Paths.
//
// A Schema is built incrementally with Set and SetConverter. It is safe to
// reuse a Schema across multiple Convert calls, but it must not be modified
// concurrently.
type Schema struct {
	root *schemaNode
}

// NewSchema returns an empty Schema.
func NewSchema() *Schema {
	return &Schema{root: &schemaNode{}}
}

// Set declares the target scalar type at the given path.
//
// It returns the schema for chaining.
func (s *Schema) Set(p Path, kind ScalarKind) *Schema {
	node := s.root
	for _, step := range p {
		node = node.child(step)
	}
	node.leaf = true
	node.kind = kind
	return s
}

// SetConverter registers a custom Converter at the given path.
//
// The custom converter is used instead of the default strict scalar
// conversion for that leaf. It returns the schema for chaining.
func (s *Schema) SetConverter(p Path, c Converter) *Schema {
	node := s.root
	for _, step := range p {
		node = node.child(step)
	}
	node.leaf = true
	node.custom = c
	return s
}

// schemaNode is an internal trie node. A node is either a leaf (a scalar
// target) or a container (a map or slice). Intermediate nodes are never
// leaves.
type schemaNode struct {
	leaf     bool
	kind     ScalarKind
	custom   Converter
	children map[string]*schemaNode
	indices  map[int]*schemaNode
}

func (n *schemaNode) child(step PathStep) *schemaNode {
	if step.IsIndex {
		if n.indices == nil {
			n.indices = map[int]*schemaNode{}
		}
		child := n.indices[step.Index]
		if child == nil {
			child = &schemaNode{}
			n.indices[step.Index] = child
		}
		return child
	}

	if n.children == nil {
		n.children = map[string]*schemaNode{}
	}
	child := n.children[step.Key]
	if child == nil {
		child = &schemaNode{}
		n.children[step.Key] = child
	}
	return child
}

func (n *schemaNode) isEmpty() bool {
	return !n.leaf && len(n.children) == 0 && len(n.indices) == 0
}

// Errors produced by the strict converter.
var (
	// errAmbiguousBool is returned when a value cannot be unambiguously
	// converted to a bool (e.g. a number or a "0"/"1" string).
	errAmbiguousBool = errors.New("ambiguous boolean conversion")

	// errLossy is returned when a conversion would lose information (e.g.
	// converting a fractional number to an integer).
	errLossy = errors.New("lossy conversion")

	// errOverflow is returned when a value is outside the range of the
	// target type.
	errOverflow = errors.New("value out of range")

	// errMissingKey is returned when a declared map key is absent from the
	// input.
	errMissingKey = errors.New("missing key")

	// errMissingIndex is returned when a declared slice index is out of
	// range.
	errMissingIndex = errors.New("missing index")

	// errNilContainer is returned when the schema expects a container but
	// the input is nil.
	errNilContainer = errors.New("nil value where container expected")

	// errContainerExpected is returned when the schema expects a container
	// but the input is a scalar.
	errContainerExpected = errors.New("expected map or slice")
)

// Convert recursively converts nested maps and slices according to the
// Schema, returning a new object and the complete list of conversion
// errors.
//
// Strict mode rejects lossy integer conversions, boolean ambiguity and
// out-of-range values. A nil value at a declared path is preserved as nil;
// a missing declared path is reported as an error. The result is only
// published when every declared path converts successfully; otherwise the
// returned object is nil and the error list holds every failure.
func Convert(input any, schema *Schema) (any, []ConversionError) {
	if schema == nil || schema.root == nil || schema.root.isEmpty() {
		return copyValue(input), nil
	}

	input, _ = indirect(input)

	var errs []ConversionError
	result := convertNode(input, schema.root, nil, &errs)

	if len(errs) > 0 {
		return nil, errs
	}
	return result, nil
}

// convertNode converts the value at a single schema node.
func convertNode(input any, node *schemaNode, path Path, errs *[]ConversionError) any {
	if node.leaf {
		converted, err := convertScalar(input, node)
		if err != nil {
			*errs = append(*errs, ConversionError{Path: append(Path{}, path...), Err: err})
			return nil
		}
		return converted
	}

	if node.isEmpty() {
		return copyValue(input)
	}

	input, _ = indirect(input)
	return convertContainer(input, node, path, errs)
}

// convertContainer converts a map or slice node.
func convertContainer(input any, node *schemaNode, path Path, errs *[]ConversionError) any {
	if input == nil {
		*errs = append(*errs, ConversionError{Path: append(Path{}, path...), Err: errNilContainer})
		return nil
	}

	rv := reflect.ValueOf(input)
	switch rv.Kind() {
	case reflect.Map:
		return convertMap(rv, node, path, errs)
	case reflect.Slice:
		return convertSlice(rv, node, path, errs)
	case reflect.Array:
		return convertArray(rv, node, path, errs)
	default:
		*errs = append(*errs, ConversionError{Path: append(Path{}, path...), Err: errContainerExpected})
		return copyValue(input)
	}
}

// convertMap converts a map node.
func convertMap(rv reflect.Value, node *schemaNode, path Path, errs *[]ConversionError) any {
	newMap := reflect.MakeMap(rv.Type())

	// Convert declared children and copy undeclared values.
	iter := rv.MapRange()
	for iter.Next() {
		keyVal := iter.Key()
		val := iter.Value()

		keyStr := keyToString(keyVal)
		child, declared := node.children[keyStr]
		if declared {
			childPath := append(append(Path{}, path...), KeyStep(keyStr))
			converted := convertNode(val.Interface(), child, childPath, errs)
			newMap.SetMapIndex(keyVal, reflect.ValueOf(converted))
		} else {
			newMap.SetMapIndex(keyVal, reflect.ValueOf(copyValue(val.Interface())))
		}
	}

	// Report missing declared keys.
	for key, child := range node.children {
		if child.isEmpty() {
			continue
		}
		keyVal := reflect.ValueOf(key)
		if !rv.MapIndex(keyVal).IsValid() {
			childPath := append(append(Path{}, path...), KeyStep(key))
			*errs = append(*errs, ConversionError{Path: childPath, Err: errMissingKey})
		}
	}

	return newMap.Interface()
}

// convertSlice converts a slice node.
func convertSlice(rv reflect.Value, node *schemaNode, path Path, errs *[]ConversionError) any {
	length := rv.Len()
	newSlice := reflect.MakeSlice(rv.Type(), length, length)

	// Convert declared indices.
	for idx, child := range node.indices {
		childPath := append(append(Path{}, path...), IndexStep(idx))
		if idx >= length {
			*errs = append(*errs, ConversionError{Path: childPath, Err: errMissingIndex})
			continue
		}
		val := rv.Index(idx)
		converted := convertNode(val.Interface(), child, childPath, errs)
		newSlice.Index(idx).Set(reflect.ValueOf(converted))
	}

	// Copy undeclared indices.
	for idx := 0; idx < length; idx++ {
		if _, declared := node.indices[idx]; !declared {
			val := rv.Index(idx)
			newSlice.Index(idx).Set(reflect.ValueOf(copyValue(val.Interface())))
		}
	}

	return newSlice.Interface()
}

// convertArray converts an array node by materializing it into a new slice
// of the same element type.
func convertArray(rv reflect.Value, node *schemaNode, path Path, errs *[]ConversionError) any {
	length := rv.Len()
	elemType := rv.Type().Elem()
	newSlice := reflect.MakeSlice(reflect.SliceOf(elemType), length, length)

	for idx, child := range node.indices {
		childPath := append(append(Path{}, path...), IndexStep(idx))
		if idx >= length {
			*errs = append(*errs, ConversionError{Path: childPath, Err: errMissingIndex})
			continue
		}
		val := rv.Index(idx)
		converted := convertNode(val.Interface(), child, childPath, errs)
		newSlice.Index(idx).Set(reflect.ValueOf(converted))
	}

	for idx := 0; idx < length; idx++ {
		if _, declared := node.indices[idx]; !declared {
			val := rv.Index(idx)
			newSlice.Index(idx).Set(reflect.ValueOf(copyValue(val.Interface())))
		}
	}

	return newSlice.Interface()
}

// convertScalar converts a scalar value at a leaf node.
func convertScalar(input any, node *schemaNode) (any, error) {
	if node.custom != nil {
		return node.custom(input)
	}

	input, _ = indirect(input)

	if input == nil {
		return nil, nil
	}

	switch node.kind {
	case KindString:
		return ToStringE(input)
	case KindBool:
		return strictBool(input)
	case KindInt:
		return strictInt(input, KindInt)
	case KindInt8:
		return strictInt(input, KindInt8)
	case KindInt16:
		return strictInt(input, KindInt16)
	case KindInt32:
		return strictInt(input, KindInt32)
	case KindInt64:
		return strictInt(input, KindInt64)
	case KindUint:
		return strictInt(input, KindUint)
	case KindUint8:
		return strictInt(input, KindUint8)
	case KindUint16:
		return strictInt(input, KindUint16)
	case KindUint32:
		return strictInt(input, KindUint32)
	case KindUint64:
		return strictInt(input, KindUint64)
	case KindFloat32:
		return strictFloat(input, 32)
	case KindFloat64:
		return strictFloat(input, 64)
	default:
		return nil, fmt.Errorf("unknown scalar kind: %d", node.kind)
	}
}

// strictBool converts a value to a bool, rejecting ambiguous inputs.
//
// Only actual bool values and the strings "true"/"false" (case-insensitive)
// are accepted. Numbers and "0"/"1" strings are rejected as ambiguous.
func strictBool(input any) (any, error) {
	switch v := input.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(v) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("%w: %q", errAmbiguousBool, v)
		}
	default:
		return nil, fmt.Errorf("%w: cannot convert %T to bool", errAmbiguousBool, input)
	}
}

// strictInt converts a value to an integer type of the given kind,
// rejecting lossy conversions and out-of-range values.
func strictInt(input any, kind ScalarKind) (any, error) {
	if _, ok := input.(bool); ok {
		return nil, fmt.Errorf("%w: cannot convert bool to integer", errAmbiguousBool)
	}

	bits, unsigned := kindBits(kind)

	rv := reflect.ValueOf(input)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return checkIntRange(rv.Int(), kind, bits, unsigned)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return checkUintRange(rv.Uint(), kind, bits, unsigned)
	case reflect.Float32, reflect.Float64:
		return checkFloatInt(rv.Float(), kind, bits, unsigned)
	case reflect.String:
		return parseIntStrict(rv.String(), kind, bits, unsigned)
	default:
		if resolved, ok := resolveAlias(input); ok {
			return strictInt(resolved, kind)
		}
		return nil, fmt.Errorf("cannot convert %T to integer", input)
	}
}

// kindBits returns the bit size and signedness for a scalar kind.
func kindBits(kind ScalarKind) (bits int, unsigned bool) {
	switch kind {
	case KindInt:
		return strconv.IntSize, false
	case KindInt8:
		return 8, false
	case KindInt16:
		return 16, false
	case KindInt32:
		return 32, false
	case KindInt64:
		return 64, false
	case KindUint:
		return strconv.IntSize, true
	case KindUint8:
		return 8, true
	case KindUint16:
		return 16, true
	case KindUint32:
		return 32, true
	case KindUint64:
		return 64, true
	default:
		return 64, false
	}
}

// strictFloat converts a value to a float type of the given bit size,
// rejecting out-of-range values.
func strictFloat(input any, bits int) (any, error) {
	if _, ok := input.(bool); ok {
		return nil, fmt.Errorf("%w: cannot convert bool to float", errAmbiguousBool)
	}

	rv := reflect.ValueOf(input)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return checkFloatRange(float64(rv.Int()), bits)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return checkFloatRange(float64(rv.Uint()), bits)
	case reflect.Float32, reflect.Float64:
		return checkFloatRange(rv.Float(), bits)
	case reflect.String:
		return parseFloatStrict(rv.String(), bits)
	default:
		if resolved, ok := resolveAlias(input); ok {
			return strictFloat(resolved, bits)
		}
		return nil, fmt.Errorf("cannot convert %T to float", input)
	}
}

// checkIntRange validates a signed integer value against the target range.
func checkIntRange(v int64, kind ScalarKind, bits int, unsigned bool) (any, error) {
	if unsigned {
		if v < 0 {
			return nil, fmt.Errorf("%w: negative value not allowed for unsigned", errOverflow)
		}
		maxVal := uint64(1)<<bits - 1
		if uint64(v) > maxVal {
			return nil, fmt.Errorf("%w: %d does not fit in uint%d", errOverflow, v, bits)
		}
		return toUintKind(uint64(v), kind), nil
	}

	minVal := int64(-1) << (bits - 1)
	maxVal := int64(1)<<(bits-1) - 1
	if v < minVal || v > maxVal {
		return nil, fmt.Errorf("%w: %d does not fit in int%d", errOverflow, v, bits)
	}
	return toIntKind(v, kind), nil
}

// checkUintRange validates an unsigned integer value against the target range.
func checkUintRange(v uint64, kind ScalarKind, bits int, unsigned bool) (any, error) {
	if !unsigned {
		maxVal := int64(1)<<(bits-1) - 1
		if v > uint64(maxVal) {
			return nil, fmt.Errorf("%w: %d does not fit in int%d", errOverflow, v, bits)
		}
		return toIntKind(int64(v), kind), nil
	}

	maxVal := uint64(1)<<bits - 1
	if v > maxVal {
		return nil, fmt.Errorf("%w: %d does not fit in uint%d", errOverflow, v, bits)
	}
	return toUintKind(v, kind), nil
}

// checkFloatInt validates a float value for integer conversion, rejecting
// fractional parts (lossy) and out-of-range values.
func checkFloatInt(v float64, kind ScalarKind, bits int, unsigned bool) (any, error) {
	if math.Trunc(v) != v {
		return nil, fmt.Errorf("%w: %v has a fractional part", errLossy, v)
	}

	if unsigned {
		if v < 0 {
			return nil, fmt.Errorf("%w: negative value not allowed for unsigned", errOverflow)
		}
		// Compare against 2^bits (exactly representable as float64) rather
		// than 2^bits-1, which loses precision at large bit sizes.
		if v >= float64(uint64(1)<<bits) {
			return nil, fmt.Errorf("%w: %v does not fit in uint%d", errOverflow, v, bits)
		}
		return toUintKind(uint64(v), kind), nil
	}

	minVal := int64(-1) << (bits - 1)
	if v < float64(minVal) || v >= float64(uint64(1)<<(bits-1)) {
		return nil, fmt.Errorf("%w: %v does not fit in int%d", errOverflow, v, bits)
	}
	return toIntKind(int64(v), kind), nil
}

// checkFloatRange validates a float value against the target float range.
func checkFloatRange(v float64, bits int) (any, error) {
	if bits == 32 {
		if v > math.MaxFloat32 || v < -math.MaxFloat32 {
			return nil, fmt.Errorf("%w: %v does not fit in float32", errOverflow, v)
		}
		return float32(v), nil
	}
	return v, nil
}

// parseIntStrict parses a string as an integer, rejecting decimal points
// (lossy) and out-of-range values.
func parseIntStrict(s string, kind ScalarKind, bits int, unsigned bool) (any, error) {
	if unsigned {
		v, err := strconv.ParseUint(s, 0, bits)
		if err == nil {
			return toUintKind(v, kind), nil
		}

		// Fall back to float parse to detect lossy decimals and range errors.
		f, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return nil, err
		}
		if math.Trunc(f) != f {
			return nil, fmt.Errorf("%w: %q has a fractional part", errLossy, s)
		}
		if f < 0 {
			return nil, fmt.Errorf("%w: negative value not allowed for unsigned", errOverflow)
		}
		// Compare against 2^bits (exactly representable as float64) rather
		// than 2^bits-1, which loses precision at large bit sizes.
		if f >= float64(uint64(1)<<bits) {
			return nil, fmt.Errorf("%w: %q does not fit in uint%d", errOverflow, s, bits)
		}
		return toUintKind(uint64(f), kind), nil
	}

	v, err := strconv.ParseInt(s, 0, bits)
	if err == nil {
		return toIntKind(v, kind), nil
	}

	// Fall back to float parse to detect lossy decimals and range errors.
	f, err2 := strconv.ParseFloat(s, 64)
	if err2 != nil {
		return nil, err
	}
	if math.Trunc(f) != f {
		return nil, fmt.Errorf("%w: %q has a fractional part", errLossy, s)
	}
	minVal := int64(-1) << (bits - 1)
	if f < float64(minVal) || f >= float64(uint64(1)<<(bits-1)) {
		return nil, fmt.Errorf("%w: %q does not fit in int%d", errOverflow, s, bits)
	}
	return toIntKind(int64(f), kind), nil
}

// parseFloatStrict parses a string as a float, rejecting out-of-range values.
func parseFloatStrict(s string, bits int) (any, error) {
	v, err := strconv.ParseFloat(s, bits)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return nil, fmt.Errorf("%w: %q does not fit in float%d", errOverflow, s, bits)
		}
		return nil, err
	}
	if bits == 32 {
		return float32(v), nil
	}
	return v, nil
}

// toIntKind converts a signed integer value to the target scalar kind.
func toIntKind(v int64, kind ScalarKind) any {
	switch kind {
	case KindInt8:
		return int8(v)
	case KindInt16:
		return int16(v)
	case KindInt32:
		return int32(v)
	case KindInt64:
		return v
	default:
		return int(v)
	}
}

// toUintKind converts an unsigned integer value to the target scalar kind.
func toUintKind(v uint64, kind ScalarKind) any {
	switch kind {
	case KindUint8:
		return uint8(v)
	case KindUint16:
		return uint16(v)
	case KindUint32:
		return uint32(v)
	case KindUint64:
		return v
	default:
		return uint(v)
	}
}

// copyValue returns a deep copy of maps and slices, leaving scalar values
// unchanged.
func copyValue(v any) any {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		newMap := reflect.MakeMap(rv.Type())
		iter := rv.MapRange()
		for iter.Next() {
			newMap.SetMapIndex(iter.Key(), reflect.ValueOf(copyValue(iter.Value().Interface())))
		}
		return newMap.Interface()
	case reflect.Slice:
		newSlice := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			newSlice.Index(i).Set(reflect.ValueOf(copyValue(rv.Index(i).Interface())))
		}
		return newSlice.Interface()
	case reflect.Array:
		newArray := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.Len(); i++ {
			newArray.Index(i).Set(reflect.ValueOf(copyValue(rv.Index(i).Interface())))
		}
		return newArray.Interface()
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return copyValue(rv.Elem().Interface())
	default:
		return v
	}
}

// keyToString converts a map key to its string representation for schema
// lookup.
func keyToString(key reflect.Value) string {
	if key.Kind() == reflect.String {
		return key.String()
	}
	return fmt.Sprintf("%v", key.Interface())
}
