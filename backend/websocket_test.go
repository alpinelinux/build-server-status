package backend

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	// "github.com/stretchr/testify/assert"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type channels struct {
	msg  chan Message
	sent chan Message
}

func TestPublisherForwardsSingleMessage(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	publisher, channels, cancel := createPublisher()

	publisher.connChan <- MockConnection{Sent: channels.sent}
	publisher.makeStep()

	channels.msg <- MessageFromString("build/message-builder", "this is a message")
	publisher.makeStep()

	cancel()

	messages := drainChannel(channels.sent)
	require.Len(messages, 1)
	assert.Equal("message-builder: this is a message", messages[0].Get())
	assert.Equal("message-builder", messages[0].BuilderName())
}

func TestPublisherSendsLastThreeMessagesPerBuilder(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	publisher, channels, cancel := createPublisher()

	msgsBuilderA := []Message{
		MessageFromString("build/BuilderA", "pulling git"),
		MessageFromString("build/BuilderA", "ugprading system"),
		MessageFromString("build/BuilderA", "uploading packages to community"),
		MessageFromString("build/BuilderA", "1/2 1/2 packageA 1.0.0-r0"),
	}

	msgsBuilderB := []Message{
		MessageFromString("build/BuilderB", "ugprading system"),
		MessageFromString("build/BuilderB", "uploading packages to community"),
		MessageFromString("build/BuilderB", "1/2 1/2 packageA 1.0.0-r0"),
		MessageFromString("build/BuilderB", "2/2 2/2 packageB 1.2.3-r0"),
	}

	for _, msg := range msgsBuilderA {
		channels.msg <- msg
		publisher.makeStep()
	}

	for _, msg := range msgsBuilderB {
		channels.msg <- msg
		publisher.makeStep()
	}

	publisher.connChan <- MockConnection{Sent: channels.sent}
	publisher.makeStep()

	receivedMessagesPerBuilder := map[string][]Message{}
	for _, msg := range drainChannel(channels.sent) {
		receivedMessagesPerBuilder[msg.BuilderName()] = append(receivedMessagesPerBuilder[msg.BuilderName()], msg)
	}

	cancel()

	require.Len(receivedMessagesPerBuilder["BuilderA"], 3, "Expected to receive 3 messages for builderA")
	require.Len(receivedMessagesPerBuilder["BuilderB"], 3, "Expected to receive 3 messages for builderB")

	for n, msg := range msgsBuilderA[1:] {
		assert.Equalf(msg, receivedMessagesPerBuilder["BuilderA"][n], "Expected message %d for builder BuilderA to be equal", n)
	}
	for n, msg := range msgsBuilderB[1:] {
		assert.Equalf(msg, receivedMessagesPerBuilder["BuilderB"][n], "Expected message %d for builder BuilderB to be equal", n)
	}
}

func TestPublisherSendsError(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	publisher, channels, cancel := createPublisher()

	channels.msg <- MessageFromString("build/BuilderA/errors", `{"reponame": "community", "pkgname": "packageA", "hostname": "builderA"}`)
	publisher.makeStep()

	publisher.connChan <- MockConnection{Sent: channels.sent}
	publisher.makeStep()

	msgs := drainChannel(channels.sent)

	cancel()

	require.Len(msgs, 1)
	assert.IsType(BuildErrorMessage{}, msgs[0])
}

func TestPublisherSendsEmptyError(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	publisher, channels, cancel := createPublisher()

	channels.msg <- MessageFromString("build/BuilderA/errors", "")
	publisher.makeStep()

	publisher.connChan <- MockConnection{Sent: channels.sent}
	publisher.makeStep()

	msgs := drainChannel(channels.sent)

	cancel()

	require.Len(msgs, 1)
	assert.IsType(BuildErrorMessage{}, msgs[0])
	assert.Equal("BuilderA: ", msgs[0].Get())
}

func TestPublisherSendsErrorMessageWhenThreeMessagesReceived(t *testing.T) {
	require := require.New(t)

	publisher, channels, cancel := createPublisher()

	channels.msg <- MessageFromString("build/BuilderA/errors", `{"reponame": "community", "pkgname": "packageA", "hostname": "builderA"}`)
	publisher.makeStep()

	msgsBuilderA := []Message{
		MessageFromString("build/BuilderA", "pulling git"),
		MessageFromString("build/BuilderA", "ugprading system"),
		MessageFromString("build/BuilderA", "uploading packages to community"),
	}

	for _, msg := range msgsBuilderA {
		channels.msg <- msg
		publisher.makeStep()
	}

	publisher.connChan <- MockConnection{Sent: channels.sent}
	publisher.makeStep()

	msgs := drainChannel(channels.sent)

	cancel()

	require.Len(msgs, 4)
	var errMsg *BuildErrorMessage
	for _, msg := range msgs {
		if m, ok := msg.(BuildErrorMessage); ok {
			errMsg = &m
			break
		}
	}
	require.NotNil(errMsg, "No error message found in received messages")
}

func TestPublisherResetsBuilderStateAfterIdle(t *testing.T) {
	require := require.New(t)

	publisher, channels, cancel := createPublisher()

	channels.msg <- MessageFromString("build/BuilderA/errors", `{"reponame": "community", "pkgname": "packageA", "hostname": "BuilderA"}`)
	publisher.makeStep()

	msgsBuilderA := []Message{
		MessageFromString("build/BuilderA", "upgrading system"),
		MessageFromString("build/BuilderA", "uploading packages to community"),
		MessageFromString("build/BuilderA", "idle"),
	}

	for _, msg := range msgsBuilderA {
		channels.msg <- msg
		publisher.makeStep()
	}

	publisher.connChan <- MockConnection{Sent: channels.sent}
	publisher.makeStep()

	msgs := drainChannel(channels.sent)

	cancel()

	require.Lenf(msgs, 1, "Expected only an idle message, received %d messages", len(msgs))

}

func createPublisher() (*BuildStatusPublisher, *channels, context.CancelFunc) {
	zerolog.SetGlobalLevel(zerolog.FatalLevel)
	channels := channels{
		msg:  make(chan Message, 1),
		sent: make(chan Message, 32),
	}

	publisher := NewBuildStatusPublisher(channels.msg)
	publisher.stepChan = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	go publisher.PublishBuildStatus(ctx)

	return publisher, &channels, cancel
}

func drainChannel(c chan Message) (msgs []Message) {

loop:
	for {
		select {
		case msg := <-c:
			msgs = append(msgs, msg)
		default:
			break loop
		}
	}

	return
}

func getMessages(channels *channels) map[string][]Message {
	return map[string][]Message{}
}

type MockConnection struct {
	Sent chan Message
}

func (c MockConnection) WriteJSON(v any) error {
	c.Sent <- v.(Message)
	return nil
}

func (c MockConnection) WriteControl(messageType int, data []byte, deadline time.Time) error {
	return nil
}

func (c MockConnection) RemoteAddr() net.Addr {
	return net.TCPAddrFromAddrPort(netip.MustParseAddrPort("192.0.2.0:12345"))
}

func (c MockConnection) Close() error {
	close(c.Sent)
	return nil
}
