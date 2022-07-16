package backend

import (
	"context"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func Run(ctx context.Context, client mqtt.Client) error {
	if t := client.Connect(); t.Wait() && t.Error() != nil {
		fmt.Printf("err: %s\n", t.Error())
	}

	msgs := make(chan Message)

	if t := client.Subscribe("build/#", 0, MessageHandler(ctx, msgs)); t.Wait() && t.Error() != nil {
		return fmt.Errorf("error subscribing to topic: %w", t.Error())
	}

	for msg := range msgs {
		fmt.Println(msg.Get())
	}

	<-ctx.Done()

	return ctx.Err()
}
