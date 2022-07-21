package backend

import (
	"context"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
)

func Run(ctx context.Context, client mqtt.Client) error {
	if t := client.Connect(); t.Wait() && t.Error() != nil {
		return fmt.Errorf("error connecting to broker: %w", t.Error())
	}

	msgs := make(chan Message, 16)

	if t := client.Subscribe("build/#", 0,
		MessageHandler(
			ctx,
			msgs,
		)); t.Wait() && t.Error() != nil {
		return fmt.Errorf("error subscribing to topic: %w", t.Error())
	}

	ctxPublisher, cancelPublisher := context.WithCancel(ctx)
	defer cancelPublisher()

	publisher := NewBuildStatusPublisher(msgs)
	go publisher.PublishBuildStatus(ctxPublisher)

	log.Info().Msg("Server started")

	<-ctx.Done()

	return ctx.Err()
}
