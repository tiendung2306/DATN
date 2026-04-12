package port

import "app/coordination"

// Clock abstracts time for timeouts and heartbeats.
type Clock interface {
	coordination.Clock
}
