package backend

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEConnectionWritesMessageEvent(t *testing.T) {
	recorder := httptest.NewRecorder()
	conn := &sseConnection{
		writer:  recorder,
		flusher: recorder,
	}

	msg := MessageFromString("build/BuilderA", "pulling git")

	err := conn.WriteJSON(msg)
	require.NoError(t, err)

	var payload map[string]any
	body := recorder.Body.String()
	require.Contains(t, body, "data: ")
	require.Contains(t, body, "\n\n")

	rawJSON := body[len("data: ") : len(body)-2]
	require.NoError(t, json.Unmarshal([]byte(rawJSON), &payload))

	assert.Equal(t, "msg", payload["MsgType"])
	assert.Equal(t, "pulling git", payload["Msg"])
	assert.Equal(t, "BuilderA", payload["Builder"])
}
