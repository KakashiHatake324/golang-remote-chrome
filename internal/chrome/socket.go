package chrome

import (
	"github.com/gorilla/websocket"
)

func NewSocket(wsUrl string) (*websocket.Conn, error) {
	ws, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		return nil, err
	}

	return ws, nil
}
