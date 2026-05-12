//go:build business_integration

// BI-022 partial — known peers list has no duplicate peer IDs (best-effort vs matrix “merge verified”).

package service

import (
	"strings"
	"testing"
)

func TestBusinessP1_Sprint5_BI022_GetKnownPeers_NoDuplicateIDs(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	peers := rt.GetKnownPeers()
	seen := make(map[string]bool)
	for _, p := range peers {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		if seen[id] {
			t.Fatalf("duplicate peer id %q in GetKnownPeers", id)
		}
		seen[id] = true
	}
}
