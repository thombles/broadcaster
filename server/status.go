package main

import (
	"code.octet-stream.net/broadcaster/protocol"
	"sync"
)

type ServerStatus struct {
	statuses      map[int]protocol.StatusMessage
	statusesMutex sync.Mutex
	changeWait    chan bool
}

var status ServerStatus

func InitServerStatus() {
	status = ServerStatus{
		statuses:   make(map[int]protocol.StatusMessage),
		changeWait: make(chan bool),
	}
}

func (s *ServerStatus) MergeStatus(radioId int, status protocol.StatusMessage) {
	s.statusesMutex.Lock()
	defer s.statusesMutex.Unlock()
	s.statuses[radioId] = status
	s.TriggerChange()
}

func (s *ServerStatus) RadioDisconnected(radioId int) {
	s.statusesMutex.Lock()
	defer s.statusesMutex.Unlock()
	delete(s.statuses, radioId)
	s.TriggerChange()
}

func (s *ServerStatus) TriggerChange() {
	close(s.changeWait)
	s.changeWait = make(chan bool)
}

func (s *ServerStatus) Statuses() map[int]protocol.StatusMessage {
	s.statusesMutex.Lock()
	defer s.statusesMutex.Unlock()
	c := make(map[int]protocol.StatusMessage)
	for k, v := range s.statuses {
		c[k] = v
	}
	return c
}

func (s *ServerStatus) ChangeChannel() chan bool {
	return s.changeWait
}
