package backend

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"testing"
	"time"

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

func TestServeHTTPShutsDownOnContextCancel(t *testing.T) {
	publisher := NewBuildStatusPublisher(make(chan Message, 1))

	listener := &blockingListener{
		addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080},
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- publisher.serveHTTP(ctx, listener)
	}()

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("serveHTTP did not shut down after context cancellation")
	}

	assert.True(t, listener.closed, "listener was not closed during shutdown")
}

type blockingListener struct {
	addr   net.Addr
	closed bool
	done   chan struct{}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	if l.done == nil {
		l.done = make(chan struct{})
	}
	<-l.done
	return nil, net.ErrClosed
}

func (l *blockingListener) Close() error {
	if l.done == nil {
		l.done = make(chan struct{})
	}
	if !l.closed {
		l.closed = true
		close(l.done)
	}
	return nil
}

func (l *blockingListener) Addr() net.Addr {
	return l.addr
}
