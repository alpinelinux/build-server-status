package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSystemMessage(t *testing.T) {
	msg := NewSystemMessage("mqtt-connected", "Connected to broker")

	assert.Equal(t, "system", msg.MsgType)
	assert.Equal(t, "mqtt-connected", msg.Status)
	assert.Equal(t, "Connected to broker", msg.Msg)
	assert.Equal(t, "", msg.BuilderName())
	assert.Equal(t, ": Connected to broker", msg.Get())
}
