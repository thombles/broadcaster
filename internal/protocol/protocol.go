package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	StartTimeFormat = "2006-01-02T15:04"
	LocalTimeFormat = "Mon _2 Jan 2006 15:04:05"

	// Radio to server

	AuthenticateType = "authenticate"
	StatusType       = "status"

	// Server to radio

	FilesType     = "files"
	PlaylistsType = "playlists"
	StopType      = "stop"

	// Status values

	StatusIdle         = "idle"
	StatusDelay        = "delay"
	StatusChannelInUse = "channel_in_use"
	StatusPlaying      = "playing"
)

// Base message type to determine what type of payload is expected.
type Message struct {
	T string
}

// Initial message from Radio to authenticate itself with a token string.
type AuthenticateMessage struct {
	T     string
	Token string
}

// Server updates the radio with the list of files that currently exist.
// This will be provided on connect and when there are any changes.
// The radio is expected to obtain all these files and cache them locally.
type FilesMessage struct {
	T     string
	Files []FileSpec
}

type PlaylistsMessage struct {
	T         string
	Playlists []PlaylistSpec
}

// Any playlist currently being played should be stopped and PTT disengaged.
type StopMessage struct {
	T string
}

type StatusMessage struct {
	T string

	// Status w.r.t. playing a playlist
	Status string

	// File being played or about to be played - empty string in idle status
	Filename string

	// Name of playlist being played - empty string in idle status
	Playlist string

	// Seconds until playback begins - never mind latency
	DelaySecondsRemaining int

	// Number of seconds file has been actually playing
	PlaybackSecondsElapsed int

	// Number of seconds waiting for channel to clear
	WaitingForChannelSeconds int

	PTT         bool
	COS         bool
	FilesInSync bool

	// Timestamp of the current time on this radio, using LocalTimeFormat
	LocalTime string

	// Time zone in use, e.g. "Australia/Hobart"
	TimeZone string
}

// Description of an individual file available in the broadcasting system.
type FileSpec struct {
	// Filename, e.g. "broadcast.wav"
	Name string
	// SHA-256 hash of the file's contents
	Hash string
}

type PlaylistSpec struct {
	Id        int
	Name      string
	StartTime string
	Entries   []EntrySpec
}

type EntrySpec struct {
	Filename     string
	DelaySeconds int
	IsRelative   bool
}

func ParseMessage(data []byte) (string, interface{}, error) {
	var t Message
	err := json.Unmarshal(data, &t)
	if err != nil {
		return "", nil, err
	}

	if t.T == AuthenticateType {
		var auth AuthenticateMessage
		err = json.Unmarshal(data, &auth)
		if err != nil {
			return "", nil, err
		}
		return t.T, auth, nil
	}

	if t.T == FilesType {
		var files FilesMessage
		err = json.Unmarshal(data, &files)
		if err != nil {
			return "", nil, err
		}
		return t.T, files, nil
	}

	if t.T == PlaylistsType {
		var playlists PlaylistsMessage
		err = json.Unmarshal(data, &playlists)
		if err != nil {
			return "", nil, err
		}
		return t.T, playlists, nil
	}

	if t.T == StatusType {
		var status StatusMessage
		err = json.Unmarshal(data, &status)
		if err != nil {
			return "", nil, err
		}
		return t.T, status, nil
	}

	if t.T == StopType {
		var stop StopMessage
		err = json.Unmarshal(data, &stop)
		if err != nil {
			return "", nil, err
		}
		return t.T, stop, nil
	}

	return "", nil, errors.New(fmt.Sprintf("unknown message type %v", t.T))
}
