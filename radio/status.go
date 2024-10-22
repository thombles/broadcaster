package main

import (
	"code.octet-stream.net/broadcaster/internal/protocol"
	"encoding/json"
	"golang.org/x/net/websocket"
	"time"
)

type BeginDelayStatus struct {
	Playlist string
	Seconds  int
	Filename string
}

type BeginWaitForChannelStatus struct {
	Playlist string
	Filename string
}

type BeginPlaybackStatus struct {
	Playlist string
	Filename string
}

type StatusCollector struct {
	Websocket                   chan *websocket.Conn
	PlaylistBeginIdle           chan bool
	PlaylistBeginDelay          chan BeginDelayStatus
	PlaylistBeginWaitForChannel chan BeginWaitForChannelStatus
	PlaylistBeginPlayback       chan BeginPlaybackStatus
	PTT                         chan bool
	COS                         chan bool
	Config                      chan RadioConfig
	FilesInSync                 chan bool
}

var statusCollector = NewStatusCollector()

func NewStatusCollector() StatusCollector {
	sc := StatusCollector{
		Websocket:                   make(chan *websocket.Conn),
		PlaylistBeginIdle:           make(chan bool),
		PlaylistBeginDelay:          make(chan BeginDelayStatus),
		PlaylistBeginWaitForChannel: make(chan BeginWaitForChannelStatus),
		PlaylistBeginPlayback:       make(chan BeginPlaybackStatus),
		PTT:                         make(chan bool),
		COS:                         make(chan bool),
		Config:                      make(chan RadioConfig),
		FilesInSync:                 make(chan bool),
	}
	go runStatusCollector(sc)
	return sc
}

func runStatusCollector(sc StatusCollector) {
	config := <-sc.Config
	var msg protocol.StatusMessage
	var lastSent protocol.StatusMessage
	msg.T = protocol.StatusType
	msg.TimeZone = config.TimeZone
	msg.Status = protocol.StatusIdle
	var ws *websocket.Conn
	// Go 1.23: no need to stop tickers when finished
	var ticker = time.NewTicker(time.Second * time.Duration(30))

	for {
		select {
		case newWebsocket := <-sc.Websocket:
			ws = newWebsocket
		case <-ticker.C:
			// should always be ticking at 1 second for these
			if msg.Status == protocol.StatusDelay {
				if msg.DelaySecondsRemaining > 0 {
					msg.DelaySecondsRemaining -= 1
				}
			}
			if msg.Status == protocol.StatusChannelInUse {
				msg.WaitingForChannelSeconds += 1
			}
			if msg.Status == protocol.StatusPlaying {
				msg.PlaybackSecondsElapsed += 1
			}
		case <-sc.PlaylistBeginIdle:
			msg.Status = protocol.StatusIdle
			msg.DelaySecondsRemaining = 0
			msg.WaitingForChannelSeconds = 0
			msg.PlaybackSecondsElapsed = 0
			msg.Playlist = ""
			msg.Filename = ""
			// Update things more slowly when nothing's playing
			ticker = time.NewTicker(time.Second * time.Duration(30))
		case delay := <-sc.PlaylistBeginDelay:
			msg.Status = protocol.StatusDelay
			msg.DelaySecondsRemaining = delay.Seconds
			msg.WaitingForChannelSeconds = 0
			msg.PlaybackSecondsElapsed = 0
			msg.Playlist = delay.Playlist
			msg.Filename = delay.Filename
			// Align ticker with start of state change, make sure it's faster
			ticker = time.NewTicker(time.Second * time.Duration(1))
		case wait := <-sc.PlaylistBeginWaitForChannel:
			msg.Status = protocol.StatusChannelInUse
			msg.DelaySecondsRemaining = 0
			msg.WaitingForChannelSeconds = 0
			msg.PlaybackSecondsElapsed = 0
			msg.Playlist = wait.Playlist
			msg.Filename = wait.Filename
			ticker = time.NewTicker(time.Second * time.Duration(1))
		case playback := <-sc.PlaylistBeginPlayback:
			msg.Status = protocol.StatusPlaying
			msg.DelaySecondsRemaining = 0
			msg.WaitingForChannelSeconds = 0
			msg.PlaybackSecondsElapsed = 0
			msg.Playlist = playback.Playlist
			msg.Filename = playback.Filename
			ticker = time.NewTicker(time.Second * time.Duration(1))
		case ptt := <-sc.PTT:
			msg.PTT = ptt
		case cos := <-sc.COS:
			msg.COS = cos
		case inSync := <-sc.FilesInSync:
			msg.FilesInSync = inSync
		}
		msg.LocalTime = time.Now().Format(protocol.LocalTimeFormat)
		msg.COS = cos.COSValue()

		if msg == lastSent {
			continue
		}
		if ws != nil {
			msgJson, _ := json.Marshal(msg)
			if _, err := ws.Write(msgJson); err != nil {
				// If websocket has failed, wait 'til we get a new one
				ws = nil
			}
			lastSent = msg
		}
	}
}
