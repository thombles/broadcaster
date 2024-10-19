package main

import (
	"sync"
)

type Playlists struct {
	changeWait    chan bool
	playlistMutex sync.Mutex
}

var playlists Playlists

func InitPlaylists() {
	playlists.changeWait = make(chan bool)
}

func (p *Playlists) GetPlaylists() []Playlist {
	p.playlistMutex.Lock()
	defer p.playlistMutex.Unlock()
	return db.GetPlaylists()
}

func (p *Playlists) WatchForChanges() ([]Playlist, chan bool) {
	p.playlistMutex.Lock()
	defer p.playlistMutex.Unlock()
	return db.GetPlaylists(), p.changeWait
}

func (p *Playlists) NotifyChanges() {
	p.playlistMutex.Lock()
	defer p.playlistMutex.Unlock()
	close(p.changeWait)
	p.changeWait = make(chan bool)
}
