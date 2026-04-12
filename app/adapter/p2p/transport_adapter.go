package p2p

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const (
	CoordDirectProtocol = protocol.ID("/coordination/direct/1.0.0")
	maxDirectMsgSize    = 1 << 20 // 1 MiB
)

// LibP2PTransport implements coordination.Transport over real libp2p
// GossipSub (Publish/Subscribe) and direct streams (SendDirect).
type LibP2PTransport struct {
	host host.Host
	ps   *pubsub.PubSub

	mu       sync.Mutex
	topics   map[string]*pubsub.Topic
	cancels  map[string]context.CancelFunc
	handlers map[string]func(peer.ID, []byte)
	closed   bool
}

// NewLibP2PTransport creates a transport backed by a real libp2p host.
// The caller must have already set up the host and PubSub.
func NewLibP2PTransport(h host.Host, ps *pubsub.PubSub) *LibP2PTransport {
	t := &LibP2PTransport{
		host:     h,
		ps:       ps,
		topics:   make(map[string]*pubsub.Topic),
		cancels:  make(map[string]context.CancelFunc),
		handlers: make(map[string]func(peer.ID, []byte)),
	}
	h.SetStreamHandler(CoordDirectProtocol, t.handleDirectStream)
	return t
}

func (t *LibP2PTransport) Publish(ctx context.Context, topic string, data []byte) error {
	t.mu.Lock()
	tp, ok := t.topics[topic]
	t.mu.Unlock()
	if !ok {
		return fmt.Errorf("not subscribed to topic %q", topic)
	}
	return tp.Publish(ctx, data)
}

func (t *LibP2PTransport) Subscribe(topic string, handler func(peer.ID, []byte)) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport closed")
	}

	if _, exists := t.topics[topic]; exists {
		t.handlers[topic] = handler
		return nil
	}

	tp, err := t.ps.Join(topic)
	if err != nil {
		return fmt.Errorf("join topic %q: %w", topic, err)
	}

	sub, err := tp.Subscribe()
	if err != nil {
		tp.Close()
		return fmt.Errorf("subscribe topic %q: %w", topic, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.topics[topic] = tp
	t.cancels[topic] = cancel
	t.handlers[topic] = handler

	go t.readLoop(ctx, topic, sub)
	return nil
}

func (t *LibP2PTransport) Unsubscribe(topic string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	cancel, ok := t.cancels[topic]
	if !ok {
		return nil
	}
	cancel()
	delete(t.cancels, topic)
	delete(t.handlers, topic)

	if tp, ok := t.topics[topic]; ok {
		tp.Close()
		delete(t.topics, topic)
	}
	return nil
}

func (t *LibP2PTransport) SendDirect(ctx context.Context, to peer.ID, data []byte) error {
	stream, err := t.host.NewStream(ctx, to, CoordDirectProtocol)
	if err != nil {
		return fmt.Errorf("open stream to %s: %w", to, err)
	}
	defer stream.Close()

	_, err = stream.Write(data)
	return err
}

func (t *LibP2PTransport) LocalPeerID() peer.ID {
	return t.host.ID()
}

func (t *LibP2PTransport) ConnectedPeers() []peer.ID {
	return t.host.Network().Peers()
}

// Close shuts down all subscriptions and removes the stream handler.
func (t *LibP2PTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true

	for topic, cancel := range t.cancels {
		cancel()
		if tp, ok := t.topics[topic]; ok {
			tp.Close()
		}
	}
	t.topics = make(map[string]*pubsub.Topic)
	t.cancels = make(map[string]context.CancelFunc)
	t.handlers = make(map[string]func(peer.ID, []byte))

	t.host.RemoveStreamHandler(CoordDirectProtocol)
}

// ── Internal ─────────────────────────────────────────────────────────────────

func (t *LibP2PTransport) readLoop(ctx context.Context, topic string, sub *pubsub.Subscription) {
	defer sub.Cancel()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == t.host.ID() {
			continue
		}
		t.mu.Lock()
		handler := t.handlers[topic]
		t.mu.Unlock()

		if handler != nil {
			handler(msg.ReceivedFrom, msg.Data)
		}
	}
}

func (t *LibP2PTransport) handleDirectStream(stream network.Stream) {
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, maxDirectMsgSize))
	if err != nil {
		slog.Warn("failed to read direct stream", "from", stream.Conn().RemotePeer(), "error", err)
		return
	}

	slog.Debug("received direct coordination message",
		"from", stream.Conn().RemotePeer(), "size", len(data))

	_ = data
}
