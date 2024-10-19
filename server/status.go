package main

import (
	"code.octet-stream.net/broadcaster/protocol"
)

type ServerStatus struct {
	statuses   map[int]protocol.StatusMessage
	changeWait chan bool
}

var status ServerStatus

func InitServerStatus() {
	status = ServerStatus{
		statuses:   make(map[int]protocol.StatusMessage),
		changeWait: make(chan bool),
	}
}

func (s *ServerStatus) MergeStatus(radioId int, status protocol.StatusMessage) {
	s.statuses[radioId] = status
	s.TriggerChange()
}

func (s *ServerStatus) RadioDisconnected(radioId int) {
	delete(s.statuses, radioId)
	s.TriggerChange()
}

func (s *ServerStatus) TriggerChange() {
	close(s.changeWait)
	s.changeWait = make(chan bool)
}

func (s *ServerStatus) Statuses() map[int]protocol.StatusMessage {
	return s.statuses
}

func (s *ServerStatus) ChangeChannel() chan bool {
	return s.changeWait
}
