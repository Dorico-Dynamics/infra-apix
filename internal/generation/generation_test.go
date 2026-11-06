package generation

import (
	"sync"
	"testing"
)

// TestEveryAdvanceCounts: Current moves by one for each Advance, also when
// registrations run concurrently.
func TestEveryAdvanceCounts(t *testing.T) {
	before := Current()
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(Advance)
	}
	wg.Wait()
	if got := Current(); got != before+50 {
		t.Errorf("Current = %d, want %d", got, before+50)
	}
}
