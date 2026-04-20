package backend

import (
	"context"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
)

func Run(ctx context.Context, client mqtt.Client, msgs chan Message) error {
	if t := client.Connect(); t.Wait() && t.Error() != nil {
		return fmt.Errorf("error connecting to broker: %w", t.Error())
	}

	if t := client.Subscribe("build/#", 0,
		MessageHandler(
			ctx,
			msgs,
		)); t.Wait() && t.Error() != nil {
		return fmt.Errorf("error subscribing to topic: %w", t.Error())
	}

	publisher := NewBuildStatusPublisher(msgs)

	log.Info().Msg("Server started")

	publisher.ListenHTTP(ctx)
	return ctx.Err()
}
