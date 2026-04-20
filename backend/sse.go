package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type Connection interface {
	WriteJSON(v any) error
	WriteComment(text string) error
	RemoteAddr() net.Addr
	Close() error
}

type BuildStatus struct {
	maxMsgLen int
	msgs      []Message
	state     *BuildStateMessage
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

func (bs *BuildStatus) clearMsgs() {
	bs.msgs = nil
}

func (bs *BuildStatus) isEmpty() bool {
	return len(bs.msgs) == 0 && bs.state == nil && bs.error == nil
}

type BuildStatusPublisher struct {
	msgChan     chan Message
	connChan    chan Connection
	connCloseCh chan string
	buildStatus map[string]*BuildStatus
	subscribers map[string]Connection
	stepChan    chan struct{}
}

func NewBuildStatusPublisher(msgChan chan Message) *BuildStatusPublisher {
	connChan := make(chan Connection, 16)
	return &BuildStatusPublisher{
		msgChan:     msgChan,
		connChan:    connChan,
		connCloseCh: make(chan string, 16),
		buildStatus: map[string]*BuildStatus{},
		subscribers: map[string]Connection{},
	}
}

func (b *BuildStatusPublisher) PublishBuildStatus(ctx context.Context) {
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case msg := <-b.msgChan:
			if _, ok := b.buildStatus[msg.BuilderName()]; !ok {
				b.buildStatus[msg.BuilderName()] = &BuildStatus{
					maxMsgLen: 3,
				}
			}
			buildStatus := b.buildStatus[msg.BuilderName()]
			hadState := !buildStatus.isEmpty()

			switch m := msg.(type) {
			case BuildErrorMessage:
				if m.Msg == "" {
					buildStatus.error = nil
				} else {
					buildStatus.error = &msg
				}
			case BuildStateMessage:
				if m.State == "" {
					buildStatus.state = nil
				} else {
					if buildStatus.state != nil && *buildStatus.state == m {
						b.waitStep()
						continue
					}
					state := m
					buildStatus.state = &state
				}
			case IdleMessage:
				log.Debug().Msgf("Received idle for %s, resetting state", msg.BuilderName())
				buildStatus.msgs = []Message{msg}
				buildStatus.error = nil
			default:
				if m, ok := msg.(GenericMessage); ok && m.Msg == "" {
					buildStatus.clearMsgs()
					if buildStatus.isEmpty() {
						if !hadState {
							delete(b.buildStatus, msg.BuilderName())
							b.waitStep()
							continue
						}
						delete(b.buildStatus, msg.BuilderName())
						msg = RemovedMessage{
							GenericMessage: GenericMessage{
								MsgType: "removed",
								Builder: msg.BuilderName(),
							},
						}
						break
					}
					b.waitStep()
					continue
				}
				if !buildStatus.addMsg(msg) {
					b.waitStep()
					continue
				}
				log.Trace().Msgf("builder %s, %d messages", msg.BuilderName(), len(buildStatus.msgs))
			}

			if buildStatus.isEmpty() {
				if !hadState {
					delete(b.buildStatus, msg.BuilderName())
					b.waitStep()
					continue
				}
				delete(b.buildStatus, msg.BuilderName())
				msg = RemovedMessage{
					GenericMessage: GenericMessage{
						MsgType: "removed",
						Builder: msg.BuilderName(),
					},
				}
			} else if m, ok := msg.(BuildStateMessage); ok && m.State == "" {
				b.waitStep()
				continue
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
				if buildstatus.state != nil {
					log.Debug().Msgf("Sending state message for %s", name)
					conn.WriteJSON(*buildstatus.state)
				}
				if buildstatus.error != nil {
					log.Debug().Msgf("Sending error message for %s", name)
					conn.WriteJSON(*buildstatus.error)
				}
			}
		case addr := <-b.connCloseCh:
			log.Info().Msgf("Removing connection: %s", addr)
			delete(b.subscribers, addr)
		case <-pingTicker.C:
			for _, conn := range b.subscribers {
				if err := conn.WriteComment("ping"); err != nil {
					log.Error().Err(err).Msg("Removing connection after ping failure")
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

		b.waitStep()
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
		if _, err := fmt.Fprintf(w, ": connected\n\n"); err != nil {
			log.Error().Err(err).Msg("failed to initialize sse stream")
			return
		}
		flusher.Flush()

		b.connChan <- conn
		<-r.Context().Done()
		b.connCloseCh <- conn.RemoteAddr().String()
	}
}

func (b *BuildStatusPublisher) ListenHTTP(ctx context.Context) {
	go b.PublishBuildStatus(ctx)

	listener, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Error().Err(err).Msg("http listener failed")
		return
	}

	if err := b.serveHTTP(ctx, listener); err != nil {
		log.Error().Err(err).Msg("http listener failed")
	}
}

func (b *BuildStatusPublisher) serveHTTP(ctx context.Context, listener net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", b.sseHandler())

	server := &http.Server{
		Handler: mux,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Error().Err(err).Msg("http shutdown failed")
		}
	}()

	log.Info().Msgf("Listening on %s", listener.Addr())
	err := server.Serve(listener)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (b *BuildStatusPublisher) makeStep() {
	if b.stepChan != nil {
		b.stepChan <- struct{}{}
	}
}

func (b *BuildStatusPublisher) waitStep() {
	if b.stepChan != nil {
		log.Debug().Msg("Waiting for tick")
		<-b.stepChan
		log.Debug().Msg("Received tick")
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

func (c *sseConnection) WriteComment(text string) error {
	if _, err := fmt.Fprintf(c.writer, ": %s\n\n", text); err != nil {
		return err
	}
	c.flusher.Flush()
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
