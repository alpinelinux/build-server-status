package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type Connection interface {
	WriteJSON(v any) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	RemoteAddr() net.Addr
	Close() error
}
type BuildStatus struct {
	maxMsgLen int
	msgs      []Message
	error     *Message
}

func (bs *BuildStatus) addMsg(msg Message) bool {
	if len(bs.msgs) > 0 && bs.msgs[len(bs.msgs)-1] == msg {
		return false
	}
	bs.msgs = append(bs.msgs, msg)

	if len(bs.msgs) <= bs.maxMsgLen {
		return true
	}

	bs.msgs = bs.msgs[len(bs.msgs)-bs.maxMsgLen:]

	return true
}

type BuildStatusPublisher struct {
	msgChan     chan Message
	connChan    chan Connection
	buildStatus map[string]*BuildStatus
	subscribers map[string]Connection
	stepChan    chan struct{}
}

func NewBuildStatusPublisher(msgChan chan Message) *BuildStatusPublisher {
	connChan := make(chan Connection, 16)
	return &BuildStatusPublisher{
		msgChan:     msgChan,
		connChan:    connChan,
		buildStatus: map[string]*BuildStatus{},
		subscribers: map[string]Connection{},
	}
}

func (b *BuildStatusPublisher) PublishBuildStatus(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-b.msgChan:
			if _, ok := b.buildStatus[msg.BuilderName()]; !ok {
				b.buildStatus[msg.BuilderName()] = &BuildStatus{
					maxMsgLen: 3,
				}
			}
			buildStatus := b.buildStatus[msg.BuilderName()]

			switch msg.(type) {
			case BuildErrorMessage:
				buildStatus.error = &msg
			case IdleMessage:
				log.Debug().Msgf("Received idle for %s, resetting state", msg.BuilderName())
				buildStatus.msgs = []Message{msg}
				buildStatus.error = nil
			default:
				if !buildStatus.addMsg(msg) {
					continue
				}
				log.Trace().Msgf("builder %s, %d messages", msg.BuilderName(), len(buildStatus.msgs))
			}

			log.Debug().Msgf("%T{%s}", msg, msg.Get())
			for _, conn := range b.subscribers {
				log.Trace().Msgf("Sending message to %s", conn.RemoteAddr())
				err := conn.WriteJSON(msg)

				if err != nil {
					log.Error().Err(err).Msg("")
					delete(b.subscribers, conn.RemoteAddr().String())
				}
			}
		case conn := <-b.connChan:
			log.Info().Msgf("Received connection from: %s", conn.RemoteAddr())
			b.subscribers[conn.RemoteAddr().String()] = conn
			for name, buildstatus := range b.buildStatus {
				log.Debug().Msgf("Sending %d messages for builder %s to subscriber %s", len(buildstatus.msgs), name, conn.RemoteAddr())
				for _, msg := range buildstatus.msgs {
					log.Trace().Msgf("Sending msg: %T{%s}", msg, msg.Get())
					conn.WriteJSON(msg)
				}
				if buildstatus.error != nil {
					log.Debug().Msgf("Sending error message for %s", name)
					conn.WriteJSON(*buildstatus.error)
				}
			}
		case <-ticker.C:
			for _, conn := range b.subscribers {
				err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(1*time.Second))
				if err != nil {
					log.Error().Err(err).Msg("Removing subscriber")
					delete(b.subscribers, conn.RemoteAddr().String())
				}
			}
		case <-ctx.Done():
			log.Info().Msg("Shutting down")
			for _, conn := range b.subscribers {
				conn.Close()
			}
			return
		}

		// Used for testing purpose only
		if b.stepChan != nil {
			log.Debug().Msg("Waiting for tick")
			<-b.stepChan
			log.Debug().Msg("Received tick")
		}
	}
}

func (b *BuildStatusPublisher) websocketHandler() http.HandlerFunc {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Msg("")
			return
		}
		conn.SetCloseHandler(func(code int, text string) error {
			log.Error().Msg(fmt.Sprintf("Connection to %s closed: %s (%d)", conn.RemoteAddr(), text, code))
			return nil
		})
		b.connChan <- conn
	}
}

func (b *BuildStatusPublisher) sseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		conn := &sseConnection{
			writer:  w,
			flusher: flusher,
			remote:  remoteAddr(r),
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		b.connChan <- conn
		<-r.Context().Done()
	}
}

func (b *BuildStatusPublisher) ListenHTTP(ctx context.Context) {
	go b.PublishBuildStatus(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/", b.websocketHandler())
	mux.HandleFunc("/events", b.sseHandler())

	log.Info().Msgf("Listening on 0.0.0.0:8080")
	err := http.ListenAndServe("0.0.0.0:8080", mux)
	log.Error().Err(err).Msg("http listener failed")
}

func (b *BuildStatusPublisher) makeStep() {
	if b.stepChan != nil {
		b.stepChan <- struct{}{}
	}
}

type sseConnection struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	remote  net.Addr
}

func (c *sseConnection) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(c.writer, "data: %s\n\n", data); err != nil {
		return err
	}
	c.flusher.Flush()
	return nil
}

func (c *sseConnection) WriteControl(messageType int, data []byte, deadline time.Time) error {
	return nil
}

func (c *sseConnection) RemoteAddr() net.Addr {
	return c.remote
}

func (c *sseConnection) Close() error {
	return nil
}

func remoteAddr(r *http.Request) net.Addr {
	addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	if err == nil {
		return addr
	}

	return &net.TCPAddr{}
}
