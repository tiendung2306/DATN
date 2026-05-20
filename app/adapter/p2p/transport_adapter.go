package p2p

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

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
	// waitGroups tracks the readLoop for each topic to ensure clean shutdown.
	waitGroups map[string]*sync.WaitGroup
	// directHandler receives payloads from /coordination/direct/1.0.0 streams
	// (same bytes as published on the group GossipSub topic).
	directHandler func(peer.ID, []byte)
	closed        bool
}

// NewLibP2PTransport creates a transport backed by a real libp2p host.
// The caller must have already set up the host and PubSub.
func NewLibP2PTransport(h host.Host, ps *pubsub.PubSub) *LibP2PTransport {
	t := &LibP2PTransport{
		host:       h,
		ps:         ps,
		topics:     make(map[string]*pubsub.Topic),
		cancels:    make(map[string]context.CancelFunc),
		handlers:   make(map[string]func(peer.ID, []byte)),
		waitGroups: make(map[string]*sync.WaitGroup),
	}
	h.SetStreamHandler(CoordDirectProtocol, t.handleDirectStream)
	return t
}

// SetDirectMessageHandler registers a callback for inbound direct coordination
// streams. Pass nil to unregister (e.g. on shutdown).
func (t *LibP2PTransport) SetDirectMessageHandler(h func(peer.ID, []byte)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.directHandler = h
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

	var tp *pubsub.Topic
	var joinErr error
	for i := 0; i < 10; i++ {
		tp, joinErr = t.ps.Join(topic)
		if joinErr == nil {
			break
		}
		slog.Warn("ps.Join failed", "topic", topic, "err", joinErr, "attempt", i)
		// "topic already exists" is a known race during rapid rejoin because
		// libp2p-pubsub's Topic.Close() is async.
		errMsg := strings.ToLower(joinErr.Error())
		if i < 9 && (strings.Contains(errMsg, "topic already exists") || strings.Contains(errMsg, "already exists")) {
			t.mu.Unlock()
			time.Sleep(200 * time.Millisecond)
			t.mu.Lock()
			continue
		}
		return fmt.Errorf("join topic %q: %w", topic, joinErr)
	}

	sub, err := tp.Subscribe()
	if err != nil {
		tp.Close()
		return fmt.Errorf("subscribe topic %q: %w", topic, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	t.topics[topic] = tp
	t.cancels[topic] = cancel
	t.handlers[topic] = handler
	t.waitGroups[topic] = wg

	wg.Add(1)
	go t.readLoop(ctx, topic, sub, wg)
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
	wg := t.waitGroups[topic]
	delete(t.cancels, topic)
	delete(t.handlers, topic)
	delete(t.waitGroups, topic)

	if tp, ok := t.topics[topic]; ok {
		delete(t.topics, topic)
		t.mu.Unlock()
		if wg != nil {
			wg.Wait()
		}
		tp.Close()
		t.mu.Lock()
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
	t.directHandler = nil

	for topic, cancel := range t.cancels {
		cancel()
		wg := t.waitGroups[topic]
		if tp, ok := t.topics[topic]; ok {
			t.mu.Unlock()
			if wg != nil {
				wg.Wait()
			}
			tp.Close()
			t.mu.Lock()
		}
	}
	t.topics = make(map[string]*pubsub.Topic)
	t.cancels = make(map[string]context.CancelFunc)
	t.handlers = make(map[string]func(peer.ID, []byte))
	t.waitGroups = make(map[string]*sync.WaitGroup)

	t.host.RemoveStreamHandler(CoordDirectProtocol)
}

// ── Internal ─────────────────────────────────────────────────────────────────

func (t *LibP2PTransport) readLoop(ctx context.Context, topic string, sub *pubsub.Subscription, wg *sync.WaitGroup) {
	defer wg.Done()
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

	from := stream.Conn().RemotePeer()
	slog.Debug("received direct coordination message", "from", from, "size", len(data))

	t.mu.Lock()
	h := t.directHandler
	t.mu.Unlock()
	if h != nil {
		h(from, data)
	} else {
		slog.Warn("direct coordination message dropped: no handler registered", "from", from)
	}
}
