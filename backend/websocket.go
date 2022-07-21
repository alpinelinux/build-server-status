package backend

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type BuildStatus struct {
	maxMsgLen int
	msgs      []Message
	error     *Message
}

func (bs *BuildStatus) addMsg(msg Message) {
	bs.msgs = append(bs.msgs, msg)

	if len(bs.msgs) <= bs.maxMsgLen {
		return
	}

	bs.msgs = bs.msgs[len(bs.msgs)-bs.maxMsgLen:]
}

type BuildStatusPublisher struct {
	msgChan     chan Message
	buildStatus map[string]*BuildStatus
	subscribers map[string]*websocket.Conn
}

func NewBuildStatusPublisher(msgChan chan Message) *BuildStatusPublisher {
	return &BuildStatusPublisher{
		msgChan:     msgChan,
		buildStatus: map[string]*BuildStatus{},
		subscribers: map[string]*websocket.Conn{},
	}
}

func (b *BuildStatusPublisher) PublishBuildStatus(ctx context.Context) {
	newConnections := make(chan *websocket.Conn, 32)

	go b.listenWebsocket(newConnections)

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
			default:
				b.buildStatus[msg.BuilderName()].addMsg(msg)
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
		case conn := <-newConnections:
			log.Info().Msgf("Received connection from: %s", conn.RemoteAddr())
			b.subscribers[conn.RemoteAddr().String()] = conn
			for _, buildstatus := range b.buildStatus {
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
			for _, conn := range b.subscribers {
				conn.Close()
			}
			return
		}
		}
	}
}

func (b *BuildStatusPublisher) listenWebsocket(connections chan *websocket.Conn) {
	upgrader := websocket.Upgrader{}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Msg("")
			return
		}
		conn.SetCloseHandler(func(code int, text string) error {
			log.Error().Msg(fmt.Sprintf("Connection to %s closed: %s (%d)", conn.RemoteAddr(), text, code))
			return nil
		})
		connections <- conn
	})
	log.Info().Msgf("Listening on 0.0.0.0:8080")
	err := http.ListenAndServe("0.0.0.0:8080", nil)
	log.Error().Err(err).Msg("http listener failed")
}
