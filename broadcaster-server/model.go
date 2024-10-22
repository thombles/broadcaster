package main

type PlaylistEntry struct {
	Id           int
	Position     int
	Filename     string
	DelaySeconds int
	IsRelative   bool
}

type User struct {
	Id           int
	Username     string
	PasswordHash string
	IsAdmin      bool
}

type Playlist struct {
	Id        int
	Enabled   bool
	Name      string
	StartTime string
}

type Radio struct {
	Id    int
	Name  string
	Token string
}
