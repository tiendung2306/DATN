package port

import "app/coordination"

// Transport abstracts GossipSub + direct streams for coordination.
type Transport interface {
	coordination.Transport
}
