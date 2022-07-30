package main

import (
	"context"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.alpinelinux.org/alpine/infra/build-server-status/backend"
)

func main() {
	opts := mqtt.
		NewClientOptions().
		AddBroker("tcp://msg.alpinelinux.org:1883").
		SetClientID(fmt.Sprintf("build-server-status-%d", time.Now().UnixMicro())).
		SetAutoReconnect(true)

	client := mqtt.NewClient(
		opts,
	)

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	})

	ctx := context.Background()
	err := backend.Run(ctx, client)
	if err != nil {
		panic(err)
	}
}
