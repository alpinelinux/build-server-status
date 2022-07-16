package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog/log"
)

var (
	progressMessagePattern = regexp.MustCompile(`([0-9]+/[0-9]+) ([0-9]+/[0-9]+) ([^ ]+) ([^ ]+)`)
)

type Progress struct {
	Current int
	Total   int
}

func (p Progress) String() string {
	return fmt.Sprintf("%d/%d", p.Current, p.Total)
}

func ProgressFromString(s string) Progress {
	currentStr, totalStr, _ := strings.Cut(s, "/")

	current, _ := strconv.Atoi(currentStr)
	total, _ := strconv.Atoi(totalStr)

	return Progress{
		Current: current,
		Total:   total,
	}
}

type Message interface {
	Get() string
	BuilderName() string
}

type GenericMessage struct {
	MsgType string
	Msg     string
	Builder string
}

func MessageFromString(topic, msg string) Message {
	_, builder, _ := strings.Cut(topic, "/")

	genericMessage := GenericMessage{
		MsgType: "msg",
		Msg:     msg,
		Builder: builder,
	}

	switch {
	case strings.HasSuffix(topic, "/errors"):
		builder, _, _ := strings.Cut(genericMessage.Builder, "/")
		genericMessage.MsgType = "error"
		genericMessage.Builder = builder
		m := BuildErrorMessage{
			GenericMessage: genericMessage,
		}
		err := json.Unmarshal([]byte(msg), &m)
		if err != nil {
			log.Error().Err(err)
			return genericMessage
		}

		return m
	case progressMessagePattern.MatchString(msg):
		genericMessage.MsgType = "progress"
		submatches := progressMessagePattern.FindStringSubmatch(msg)
		m := BuildStatusMessage{
			GenericMessage: genericMessage,
			BuildProgress:  ProgressFromString(submatches[1]),
			TotalProgress:  ProgressFromString(submatches[2]),
			PackageName:    submatches[3],
			PackageVersion: submatches[4],
		}

		return m
	default:
		return genericMessage
	}
}

func (m GenericMessage) Get() string {
	return fmt.Sprintf("%s: %s", m.Builder, m.Msg)
}

func (m GenericMessage) BuilderName() string {
	return m.Builder
}

type BuildStatusMessage struct {
	GenericMessage
	BuildProgress  Progress
	TotalProgress  Progress
	PackageName    string
	PackageVersion string
}

func (m BuildStatusMessage) Get() string {
	return fmt.Sprintf("%s: %s %s %s %s", m.Builder, m.BuildProgress, m.TotalProgress, m.PackageName, m.PackageVersion)
}

type BuildErrorMessage struct {
	GenericMessage
	Reponame string
	Pkgname  string
	Logurl   string
	Hostname string
}

func MessageHandler(ctx context.Context, msgs chan Message) mqtt.MessageHandler {
	return func(c mqtt.Client, m mqtt.Message) {
		log.Debug().
			Str("topic", m.Topic()).
			Str("payload", string(m.Payload())).
			Msg("Received message from broker")
		msgs <- MessageFromString(m.Topic(), string(m.Payload()))
	}
}
