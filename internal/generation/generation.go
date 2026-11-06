// Package generation counts changes to apix's route and plugin registries.
// The runtime handler keeps a document it built while the count is unchanged.
package generation

import "sync/atomic"

var current atomic.Uint64

// Advance records a change. Call it once the change is visible to readers of
// the registry.
func Advance() { current.Add(1) }

// Current returns the number of changes so far. Read it before reading the
// registries: a change made in between then leads to one more build, never to
// a document kept without it.
func Current() uint64 { return current.Load() }
