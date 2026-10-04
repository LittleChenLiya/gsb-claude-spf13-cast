// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"fmt"
	"strings"
)

// Entry is a single key/value pair in an explicitly ordered input list.
type Entry[K any, V any] struct {
	Key   K
	Value V
}

// KeySource records the origin of a converted map key: the converted key and
// the original key it was produced from. Sources are returned in input order.
type KeySource[K comparable, K0 any] struct {
	// Key is the converted result key.
	Key K
	// Origin is the original key the result key was produced from.
	Origin K0
}

// ConvertError describes a single conversion failure at an explicit position
// in the input list.
type ConvertError struct {
	// Index is the position of the offending entry in the input list.
	Index int
	// Origin is the original value that failed to convert.
	Origin any
	// Err is the underlying conversion error.
	Err error
}

func (e *ConvertError) Error() string {
	return fmt.Sprintf("entry %d (%q): %v", e.Index, e.Origin, e.Err)
}

func (e *ConvertError) Unwrap() error { return e.Err }

// KeyCollisionError describes two distinct input keys that convert to the same
// result key.
type KeyCollisionError struct {
	// Index is the position of the later colliding entry in the input list.
	Index int
	// Origin is the original key of the later colliding entry.
	Origin any
	// ExistingIndex is the position of the first entry that claimed the result key.
	ExistingIndex int
	// ExistingOrigin is the original key of the first entry.
	ExistingOrigin any
	// Key is the converted result key both entries collide on.
	Key any
}

func (e *KeyCollisionError) Error() string {
	return fmt.Sprintf("key %d (%q) collides with key %d (%q) after conversion to %#v", e.Index, e.Origin, e.ExistingIndex, e.ExistingOrigin, e.Key)
}

// ConvertErrors is the aggregated error returned by ConvertMapE when one or
// more entries fail to convert or collide. Every problem is reported together
// with its original key position; no partial result is delivered.
type ConvertErrors []error

func (errs ConvertErrors) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d conversion error(s):", len(errs))
	for _, e := range errs {
		b.WriteString("\n  - ")
		b.WriteString(e.Error())
	}
	return b.String()
}

func (errs ConvertErrors) Unwrap() []error { return []error(errs) }

// ConvertMapE converts an explicitly ordered list of key/value pairs into a
// map[K]V.
//
// All keys are converted first, each exactly once, and checked for collisions
// using the target type's own equality. Keys that are distinct in the input
// may collide after conversion (for example "01" and "1" both convert to the
// integer 1, but not to the same string). If any key fails to convert (illegal
// type, overflow, ...) or collides with an earlier key, ConvertMapE returns a
// ConvertErrors aggregating every problem with its original key position and
// no values are converted. Only when every key converts without error or
// collision are the values converted; a value conversion failure likewise
// aborts the whole operation. The input list is never modified.
//
// The returned source table records, in input order, the converted key and the
// original key it was produced from.
func ConvertMapE[K0, V0 any, K comparable, V any](
	entries []Entry[K0, V0],
	keyFn func(K0) (K, error),
	valFn func(V0) (V, error),
) (map[K]V, []KeySource[K, K0], error) {
	// Phase 1: convert every key (each exactly once) and detect collisions
	// using the target type's equality. No values are converted until all keys
	// have been converted without error or collision.
	convertedKeys := make([]K, len(entries))
	var keyErrs ConvertErrors
	seen := make(map[K]int, len(entries)) // converted key -> index of first owner

	for i, e := range entries {
		k, err := keyFn(e.Key)
		if err != nil {
			keyErrs = append(keyErrs, &ConvertError{Index: i, Origin: e.Key, Err: err})
			continue
		}
		convertedKeys[i] = k

		if prev, ok := seen[k]; ok {
			keyErrs = append(keyErrs, &KeyCollisionError{
				Index:          i,
				Origin:         e.Key,
				ExistingIndex:  prev,
				ExistingOrigin: entries[prev].Key,
				Key:            k,
			})
			continue
		}
		seen[k] = i
	}

	if len(keyErrs) > 0 {
		return nil, nil, keyErrs
	}

	// Phase 2: convert every value. A value failure aborts the whole operation.
	values := make([]V, len(entries))
	var valErrs ConvertErrors
	for i, e := range entries {
		v, err := valFn(e.Value)
		if err != nil {
			valErrs = append(valErrs, &ConvertError{Index: i, Origin: e.Value, Err: err})
			continue
		}
		values[i] = v
	}
	if len(valErrs) > 0 {
		return nil, nil, valErrs
	}

	// Phase 3: build the result map and the key source table in input order.
	result := make(map[K]V, len(entries))
	sources := make([]KeySource[K, K0], len(entries))
	for i, e := range entries {
		k := convertedKeys[i]
		result[k] = values[i]
		sources[i] = KeySource[K, K0]{Key: k, Origin: e.Key}
	}

	return result, sources, nil
}

// ConvertMap converts an explicitly ordered list of key/value pairs into a
// map[K]V, ignoring any conversion error (a nil map is returned on error).
func ConvertMap[K0, V0 any, K comparable, V any](
	entries []Entry[K0, V0],
	keyFn func(K0) (K, error),
	valFn func(V0) (V, error),
) (map[K]V, []KeySource[K, K0]) {
	m, sources, _ := ConvertMapE(entries, keyFn, valFn)
	return m, sources
}
