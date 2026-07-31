//go:build integration

package questbridge

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

func subscribeToAgentTasks(t *testing.T, client *natsclient.Client, ctx context.Context) <-chan []byte {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream() failed: %v", err)
	}
	consumer, err := js.CreateOrUpdateConsumer(ctx, "AGENT", jetstream.ConsumerConfig{
		FilterSubject: "agent.task.>",
		DeliverPolicy: jetstream.DeliverNewPolicy,
		AckPolicy:     jetstream.AckNonePolicy,
	})
	if err != nil {
		t.Fatalf("create agent.task consumer: %v", err)
	}
	updates := make(chan []byte, 8)
	consume, err := consumer.Consume(func(msg jetstream.Msg) {
		data := append([]byte(nil), msg.Data()...)
		select {
		case updates <- data:
		default:
		}
	})
	if err != nil {
		t.Fatalf("consume agent.task: %v", err)
	}
	t.Cleanup(consume.Stop)
	return updates
}
