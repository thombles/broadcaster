package main

import (
	"code.octet-stream.net/broadcaster/internal/protocol"
	"encoding/json"
	"golang.org/x/net/websocket"
	"sync"
)

type CommandRouter struct {
	connsMutex sync.Mutex
	conns      map[int]*websocket.Conn
}

var commandRouter CommandRouter

func InitCommandRouter() {
	commandRouter.conns = make(map[int]*websocket.Conn)
}

func (c *CommandRouter) AddWebsocket(radioId int, ws *websocket.Conn) {
	c.connsMutex.Lock()
	defer c.connsMutex.Unlock()
	c.conns[radioId] = ws
}

func (c *CommandRouter) RemoveWebsocket(ws *websocket.Conn) {
	c.connsMutex.Lock()
	defer c.connsMutex.Unlock()
	key := -1
	for k, v := range c.conns {
		if v == ws {
			key = k
		}
	}
	if key != -1 {
		delete(c.conns, key)
	}

}

func (c *CommandRouter) Stop(radioId int) {
	c.connsMutex.Lock()
	defer c.connsMutex.Unlock()
	ws := c.conns[radioId]
	if ws != nil {
		stop := protocol.StopMessage{
			T: protocol.StopType,
		}
		msg, _ := json.Marshal(stop)
		ws.Write(msg)
	}
}
