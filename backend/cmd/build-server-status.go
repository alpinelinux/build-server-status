package main

import (
	"context"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gitlab.alpinelinux.org/alpine/infra/build-server-status/backend"
)

func main() {
	opts := mqtt.
		NewClientOptions().
		AddBroker("tcp://msg.alpinelinux.org:1883").
		SetClientID("build-server-status-123").
		SetAutoReconnect(true)

	client := mqtt.NewClient(
		opts,
	)

	ctx := context.Background()
	err := backend.Run(ctx, client)
	if err != nil {
		panic(err)
	}
}
