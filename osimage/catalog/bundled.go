package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed bundled.json
var bundledJSON []byte

var (
	bundledOnce sync.Once
	bundledSnap Snapshot
	bundledErr  error
)

// Bundled is the Phase 0 catalog compiled into the binary. It is what the
// server serves before the first refresh, and what OS_CATALOG_OFFLINE pins.
func Bundled() (Snapshot, error) {
	bundledOnce.Do(func() {
		if err := json.Unmarshal(bundledJSON, &bundledSnap); err != nil {
			bundledErr = fmt.Errorf("bundled catalog: %w", err)
			return
		}
		bundledSnap.Origin = OriginBundled
	})
	if bundledErr != nil {
		return Snapshot{}, bundledErr
	}
	return cloneSnapshot(bundledSnap), nil
}
