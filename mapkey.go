// Copyright © 2014 Steve Francia <spf@spf13.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cast

import (
	"errors"
	"fmt"
)

// MapEntry is one explicitly ordered input entry for ConvertMapE.
//
// The conversion order is fixed by the order of the explicit input
// list, not by map iteration order, so results are deterministic.
type MapEntry[K comparable, V any] struct {
	Key   K
	Value V
}

// ConvertedMap is the result of a successful ConvertMapE call.
type ConvertedMap[K1 comparable, K2 comparable, V2 any] struct {
	// Map holds the converted keys and values.
	Map map[K2]V2

	// Sources is the key provenance table: it maps every converted
	// key back to the original key it came from.
	Sources map[K2]K1
}

// KeyError reports a conversion failure tied to an original key
// position in the explicit input list.
type KeyError struct {
	Index int // position in the input entry list
	Key   any // original key
	Err   error
}

func (e *KeyError) Error() string {
	return fmt.Sprintf("entry %d (key %#v): %v", e.Index, e.Key, e.Err)
}

func (e *KeyError) Unwrap() error { return e.Err }

// KeyCollisionError reports that two original keys converted to the
// same target key under the target type's real equality rules.
type KeyCollisionError struct {
	Key    any // the colliding converted key
	First  int // index of the first entry producing Key
	Second int // index of the later entry producing Key
}

func (e *KeyCollisionError) Error() string {
	return fmt.Sprintf("entries %d and %d both convert to key %#v", e.First, e.Second, e.Key)
}

// ConvertMapE converts a map given as an explicit, ordered entry list
// into a map with converted keys and values.
//
// The conversion runs in two strict phases:
//
//  1. Every key is converted first (keyFn is called exactly once per
//     entry). Converted keys are compared using the target key type's
//     real equality rules (Go == semantics for K2). Any collision is
//     an error: no later value may overwrite an earlier one.
//  2. Only if every key converts cleanly and no collisions occur are
//     the values converted.
//
// Illegal types and overflows are reported as *KeyError values that
// carry the original key position; all failures are aggregated and
// returned together. The result is delivered only when everything
// succeeds; on any error the returned *ConvertedMap is nil. The input
// list is never modified.
func ConvertMapE[K1 comparable, V1 any, K2 comparable, V2 any](
	entries []MapEntry[K1, V1],
	keyFn func(K1) (K2, error),
	valFn func(V1) (V2, error),
) (*ConvertedMap[K1, K2, V2], error) {
	// Phase 1: convert all keys, checking for collisions.
	converted := make([]K2, len(entries))
	seen := make(map[K2]int, len(entries)) // converted key -> first index

	var errs []error

	for i, e := range entries {
		k, err := keyFn(e.Key)
		if err != nil {
			errs = append(errs, &KeyError{Index: i, Key: e.Key, Err: err})
			continue
		}

		if first, ok := seen[k]; ok {
			errs = append(errs, &KeyCollisionError{Key: k, First: first, Second: i})
			continue
		}

		seen[k] = i
		converted[i] = k
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Phase 2: convert all values.
	m := make(map[K2]V2, len(entries))
	sources := make(map[K2]K1, len(entries))

	for i, e := range entries {
		v, err := valFn(e.Value)
		if err != nil {
			errs = append(errs, &KeyError{Index: i, Key: e.Key, Err: err})
			continue
		}

		m[converted[i]] = v
		sources[converted[i]] = e.Key
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &ConvertedMap[K1, K2, V2]{Map: m, Sources: sources}, nil
}
