package main

import (
	"context"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"gitlab.alpinelinux.org/alpine/infra/build-server-status/backend"
)

func main() {
	var levelFlag string
	pflag.StringVarP(&levelFlag, "log-level", "l", "info", "Log level verbosity")

	pflag.Parse()

	logLevel, err := zerolog.ParseLevel(levelFlag)

	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unknown log level: %s\n", levelFlag)
		os.Exit(1)
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(logLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	})

	log.Info().Msgf("Logging with loglevel %s", logLevel)

	opts := mqtt.
		NewClientOptions().
		AddBroker("tcp://msg.alpinelinux.org:1883").
		SetClientID(fmt.Sprintf("build-server-status-%d", time.Now().UnixMicro())).
		SetAutoReconnect(true).
		SetCleanSession(false).
		SetMaxReconnectInterval(1 * time.Minute).
		SetOnConnectHandler(func(c mqtt.Client) {
			log.Info().Msg("Connected to broker")
		}).
		SetConnectionLostHandler(func(c mqtt.Client, err error) {
			log.
				Error().
				Err(fmt.Errorf("Connection to broker lost: %w", err)).
				Msg("")
		})

	client := mqtt.NewClient(
		opts,
	)

	ctx := context.Background()
	err = backend.Run(ctx, client)
	if err != nil {
		panic(err)
	}
}
