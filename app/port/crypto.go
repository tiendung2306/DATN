package port

import "app/coordination"

// CryptoEngine abstracts the Rust MLS sidecar (same contract as coordination.MLSEngine).
type CryptoEngine interface {
	coordination.MLSEngine
}
