package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSystemMessage(t *testing.T) {
	msg := NewSystemMessage("mqtt-connected", "Connected to broker")

	assert.Equal(t, "system", msg.MsgType)
	assert.Equal(t, "mqtt-connected", msg.Status)
	assert.Equal(t, "Connected to broker", msg.Msg)
	assert.Equal(t, "", msg.BuilderName())
	assert.Equal(t, ": Connected to broker", msg.Get())
}

func TestParseBuildTopic(t *testing.T) {
	tests := []struct {
		topic   string
		builder string
		subtype string
	}{
		{topic: "build/builder1", builder: "builder1", subtype: ""},
		{topic: "build/builder1/errors", builder: "builder1", subtype: "errors"},
		{topic: "build/builder1/state", builder: "builder1", subtype: "state"},
		{topic: "build", builder: "", subtype: ""},
		{topic: "build/", builder: "", subtype: ""},
		{topic: "build/builder1/errors/extra", builder: "", subtype: ""},
		{topic: "other/builder1", builder: "", subtype: ""},
	}

	for _, tt := range tests {
		builder, subtype := parseBuildTopic(tt.topic)
		assert.Equal(t, tt.builder, builder, tt.topic)
		assert.Equal(t, tt.subtype, subtype, tt.topic)
	}
}

func TestMessageFromStringParsesErrorTopicBySegments(t *testing.T) {
	msg := MessageFromString("build/BuilderA/errors", `{"reponame":"community","pkgname":"packageA","hostname":"BuilderA"}`)

	require.IsType(t, BuildErrorMessage{}, msg)
	assert.Equal(t, "BuilderA", msg.BuilderName())
}

func TestMessageFromStringKeepsBuilderNameForStateSubtopic(t *testing.T) {
	msg := MessageFromString("build/BuilderA/state", "online")

	require.IsType(t, GenericMessage{}, msg)
	assert.Equal(t, "BuilderA", msg.BuilderName())
	assert.Equal(t, "online", msg.Get()[len("BuilderA: "):])
}
