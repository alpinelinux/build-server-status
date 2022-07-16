package backend

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/eclipse/paho.mqtt.golang"
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
	case progressMessagePattern.MatchString(msg):
		genericMessage.MsgType = "progress"
		submatches := progressMessagePattern.FindStringSubmatch(msg)
		msg := BuildStatusMessage{
			GenericMessage: genericMessage,
			BuildProgress:  ProgressFromString(submatches[1]),
			TotalProgress:  ProgressFromString(submatches[2]),
			PackageName:    submatches[3],
			PackageVersion: submatches[4],
		}

		return msg
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

func MessageHandler(ctx context.Context, msgs chan Message) mqtt.MessageHandler {
	return func(c mqtt.Client, m mqtt.Message) {
		msgs <- MessageFromString(m.Topic(), string(m.Payload()))
	}
}
