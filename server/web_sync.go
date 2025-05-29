package main

import (
	"fmt"
	"html/template"
	"log"
	"sort"
	"strconv"
	"strings"

	"code.octet-stream.net/broadcaster/internal/protocol"
	"golang.org/x/net/websocket"
)

func WebSync(ws *websocket.Conn) {
	log.Println("A web user connected with WebSocket")
	buf := make([]byte, 16384)

	badRead := false
	isAuthenticated := false
	var user User
	for {
		// Ignore any massively oversize messages
		n, err := ws.Read(buf)
		if err != nil {
			if user.Username != "" {
				log.Println("Lost websocket to user:", user.Username)
			} else {
				log.Println("Lost unauthenticated website websocket")
			}
			return
		}
		if n == len(buf) {
			badRead = true
			continue
		} else if badRead {
			badRead = false
			continue
		}

		if !isAuthenticated {
			token := string(buf[:n])
			u, err := users.GetUserForSession(token)
			if err != nil {
				log.Println("Could not find user for offered token", token, err)
				ws.Close()
				return
			}
			user = u
			log.Println("User authenticated:", user.Username)
			isAuthenticated = true

			go KeepWebUpdated(ws)

			// send initial playlists message
			err = sendRadioStatusToWeb(ws)
			if err != nil {
				return
			}
		}
	}
}

type WebStatusData struct {
	Radios []WebRadioStatus
}

type WebRadioStatus struct {
	Name          string
	LocalTime     string
	TimeZone      string
	ChannelClass  string
	ChannelState  string
	Playlist      string
	File          string
	Status        string
	Id            string
	DisableCancel bool
	FilesInSync   bool
}

func sendRadioStatusToWeb(ws *websocket.Conn) error {
	webStatuses := make([]WebRadioStatus, 0)
	radioStatuses := status.Statuses()
	keys := make([]int, 0)
	for i := range radioStatuses {
		keys = append(keys, i)
	}
	sort.Ints(keys)
	for _, i := range keys {
		v := radioStatuses[i]
		radio, err := db.GetRadio(i)
		if err != nil {
			continue
		}
		var channelClass, channelState string
		if v.PTT {
			channelClass = "ptt"
			channelState = "PTT"
		} else if v.COS {
			channelClass = "cos"
			channelState = "RX"
		} else {
			channelClass = "clear"
			channelState = "CLEAR"
		}
		var statusText string
		var disableCancel bool
		if v.Status == protocol.StatusIdle {
			statusText = "Idle"
			disableCancel = true
		} else if v.Status == protocol.StatusDelay {
			statusText = fmt.Sprintf("Performing delay before transmit: %ds remain", v.DelaySecondsRemaining)
			disableCancel = false
		} else if v.Status == protocol.StatusChannelInUse {
			statusText = fmt.Sprintf("Waiting for channel to clear: %ds", v.WaitingForChannelSeconds)
			disableCancel = false
		} else if v.Status == protocol.StatusPlaying {
			statusText = fmt.Sprintf("Playing: %d:%02d", v.PlaybackSecondsElapsed/60, v.PlaybackSecondsElapsed%60)
			disableCancel = false
		}
		playlist := v.Playlist
		if playlist == "" {
			playlist = "-"
		}
		filename := v.Filename
		if filename == "" {
			filename = "-"
		}
		webStatuses = append(webStatuses, WebRadioStatus{
			Name:          radio.Name,
			LocalTime:     v.LocalTime,
			TimeZone:      v.TimeZone,
			ChannelClass:  channelClass,
			ChannelState:  channelState,
			Playlist:      playlist,
			File:          filename,
			Status:        statusText,
			Id:            strconv.Itoa(i),
			DisableCancel: disableCancel,
			FilesInSync:   v.FilesInSync,
		})
	}
	data := WebStatusData{
		Radios: webStatuses,
	}
	buf := new(strings.Builder)
	tmpl := template.Must(template.ParseFS(content, "templates/radios.partial.html"))
	tmpl.Execute(buf, data)
	_, err := ws.Write([]byte(buf.String()))
	return err
}

func KeepWebUpdated(ws *websocket.Conn) {
	for {
		<-status.ChangeChannel()
		err := sendRadioStatusToWeb(ws)
		if err != nil {
			return
		}
	}
}
