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
	builder, subtype := parseBuildTopic(topic)
	if builder == "" {
		return nil
	}

	genericMessage := GenericMessage{
		MsgType: "msg",
		Msg:     msg,
		Builder: builder,
	}

	switch subtype {
	case "errors":
		genericMessage.MsgType = "error"
		m := BuildErrorMessage{
			GenericMessage: genericMessage,
		}
		if msg != "" {
			err := json.Unmarshal([]byte(msg), &m)
			if err != nil {
				log.Error().Err(err).Msg("")
				return genericMessage
			}
		}

		return m
	case "state":
		m := BuildStateMessage{
			GenericMessage: genericMessage,
			State:          msg,
		}
		m.MsgType = "state"
		return m
	case "":
		switch {
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
		case msg == "idle":
			m := IdleMessage{GenericMessage: genericMessage}
			m.MsgType = "idle"
			return m
		default:
			return genericMessage
		}
	}

	// Ignore unknown build subtopics.
	return nil
}

func parseBuildTopic(topic string) (builder, subtype string) {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "build" || parts[1] == "" {
		return "", ""
	}

	return parts[1], strings.Join(parts[2:], "/")
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

type IdleMessage struct {
	GenericMessage
}

type BuildStateMessage struct {
	GenericMessage
	State string
}

type RemovedMessage struct {
	GenericMessage
}

type SystemMessage struct {
	GenericMessage
	Status string
}

func NewSystemMessage(status, msg string) SystemMessage {
	return SystemMessage{
		GenericMessage: GenericMessage{
			MsgType: "system",
			Msg:     msg,
		},
		Status: status,
	}
}

func MessageHandler(ctx context.Context, msgs chan Message) mqtt.MessageHandler {
	return func(c mqtt.Client, m mqtt.Message) {
		log.Debug().
			Str("topic", m.Topic()).
			Str("payload", string(m.Payload())).
			Msg("Received message from broker")
		msg := MessageFromString(m.Topic(), string(m.Payload()))
		if msg == nil {
			log.Debug().
				Str("topic", m.Topic()).
				Msg("Ignoring unknown build subtopic")
			return
		}
		msgs <- msg
	}
}
