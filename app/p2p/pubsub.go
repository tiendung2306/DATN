package p2p

import (
	"context"
	"log/slog"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const GlobalTopicName = "/org/chat/global"

type ChatRoom struct {
	Topic    *pubsub.Topic
	Sub      *pubsub.Subscription
	Messages chan *pubsub.Message
}

func JoinChatRoom(ctx context.Context, ps *pubsub.PubSub, topicName string) (*ChatRoom, error) {
	topic, err := ps.Join(topicName)
	if err != nil {
		return nil, err
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}

	cr := &ChatRoom{
		Topic:    topic,
		Sub:      sub,
		Messages: make(chan *pubsub.Message, 128),
	}

	go cr.readLoop(ctx)
	return cr, nil
}

func (cr *ChatRoom) readLoop(ctx context.Context) {
	for {
		msg, err := cr.Sub.Next(ctx)
		if err != nil {
			close(cr.Messages)
			return
		}
		
		slog.Info("Received PubSub message", 
			"from", msg.GetFrom().String(), 
			"topic", msg.GetTopic(),
			"data", string(msg.Data))
		
		cr.Messages <- msg
	}
}

func (cr *ChatRoom) Publish(ctx context.Context, message []byte) error {
	return cr.Topic.Publish(ctx, message)
}
