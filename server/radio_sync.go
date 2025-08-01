package main

import (
	"code.octet-stream.net/broadcaster/internal/protocol"
	"encoding/json"
	"golang.org/x/net/websocket"
	"log"
)

func RadioSync(ws *websocket.Conn) {
	log.Println("Radio websocket connected, not yet authenticated")
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
			commandRouter.AddWebsocket(r.Id, ws)
			defer commandRouter.RemoveWebsocket(ws)

			go KeepFilesUpdated(ws)
			go KeepPlaylistsUpdated(ws)
		}

		if t == protocol.StatusType {
			statusMsg := msg.(protocol.StatusMessage)
			log.Println("Received Status from", radio.Name, ":", statusMsg)
			status.MergeStatus(radio.Id, statusMsg)
		}
	}
}

func sendPlaylistsMessageToRadio(ws *websocket.Conn, p []Playlist) error {
	playlistSpecs := make([]protocol.PlaylistSpec, 0)
	for _, v := range p {
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
		p, ch := playlists.WatchForChanges()
		err := sendPlaylistsMessageToRadio(ws, p)
		if err != nil {
			return
		}
		<-ch
	}
}

func sendFilesMessageToRadio(ws *websocket.Conn, f []FileSpec) error {
	specs := make([]protocol.FileSpec, 0)
	for _, v := range f {
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
		f, ch := files.WatchForChanges()
		err := sendFilesMessageToRadio(ws, f)
		if err != nil {
			return
		}
		<-ch
	}
}
