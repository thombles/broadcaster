package main

import (
	"code.octet-stream.net/broadcaster/protocol"
	"encoding/json"
	"golang.org/x/net/websocket"
	"log"
)

func RadioSync(ws *websocket.Conn) {
	log.Println("A websocket connected, I think")
	buf := make([]byte, 16384)

	badRead := false
	isAuthenticated := false
	var radio Radio
	for {
		// Ignore any massively oversize messages
		n, err := ws.Read(buf)
		if err != nil {
			if radio.Name != "" {
				log.Println("Lost websocket to radio:", radio.Name)
				status.RadioDisconnected(radio.Id)
			} else {
				log.Println("Lost unauthenticated websocket")
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

		t, msg, err := protocol.ParseMessage(buf[:n])
		if err != nil {
			log.Println(err)
			return
		}

		if !isAuthenticated && t != protocol.AuthenticateType {
			continue
		}

		if t == protocol.AuthenticateType && !isAuthenticated {
			authMsg := msg.(protocol.AuthenticateMessage)
			r, err := db.GetRadioByToken(authMsg.Token)
			if err != nil {
				log.Println("Could not find radio for offered token", authMsg.Token)
			}
			radio = r
			log.Println("Radio authenticated:", radio.Name)
			isAuthenticated = true

			go KeepFilesUpdated(ws)

			// send initial file message
			err = sendFilesMessageToRadio(ws)
			if err != nil {
				return
			}

			go KeepPlaylistsUpdated(ws)

			// send initial playlists message
			err = sendPlaylistsMessageToRadio(ws)
			if err != nil {
				return
			}
		}

		if t == protocol.StatusType {
			statusMsg := msg.(protocol.StatusMessage)
			log.Println("Received Status from", radio.Name, ":", statusMsg)
			status.MergeStatus(radio.Id, statusMsg)
		}
	}
}

func sendPlaylistsMessageToRadio(ws *websocket.Conn) error {
	playlistSpecs := make([]protocol.PlaylistSpec, 0)
	for _, v := range db.GetPlaylists() {
		if v.Enabled {
			entrySpecs := make([]protocol.EntrySpec, 0)
			for _, e := range db.GetEntriesForPlaylist(v.Id) {
				entrySpecs = append(entrySpecs, protocol.EntrySpec{Filename: e.Filename, DelaySeconds: e.DelaySeconds, IsRelative: e.IsRelative})
			}
			playlistSpecs = append(playlistSpecs, protocol.PlaylistSpec{Id: v.Id, Name: v.Name, StartTime: v.StartTime, Entries: entrySpecs})
		}
	}
	playlists := protocol.PlaylistsMessage{
		T:         protocol.PlaylistsType,
		Playlists: playlistSpecs,
	}
	msg, _ := json.Marshal(playlists)
	_, err := ws.Write(msg)
	return err
}

func KeepPlaylistsUpdated(ws *websocket.Conn) {
	for {
		<-playlistChangeWait
		err := sendPlaylistsMessageToRadio(ws)
		if err != nil {
			return
		}
	}
}

func sendFilesMessageToRadio(ws *websocket.Conn) error {
	specs := make([]protocol.FileSpec, 0)
	for _, v := range files.Files() {
		specs = append(specs, protocol.FileSpec{Name: v.Name, Hash: v.Hash})
	}
	files := protocol.FilesMessage{
		T:     protocol.FilesType,
		Files: specs,
	}
	msg, _ := json.Marshal(files)
	_, err := ws.Write(msg)
	return err
}

func KeepFilesUpdated(ws *websocket.Conn) {
	for {
		<-files.ChangeChannel()
		err := sendFilesMessageToRadio(ws)
		if err != nil {
			return
		}
	}
}
