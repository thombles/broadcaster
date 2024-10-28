package main

import (
	"database/sql"
	"errors"
	"log"
	_ "modernc.org/sqlite"
	"time"
)

type Database struct {
	sqldb *sql.DB
}

var db Database

func InitDatabase() {
	sqldb, err := sql.Open("sqlite", config.SqliteDB)
	if err != nil {
		log.Fatal(err)
	}
	db.sqldb = sqldb

	_, err = db.sqldb.Exec("PRAGMA journal_mode = WAL")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.sqldb.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.sqldb.Exec("PRAGMA busy_timeout = 5000")
	if err != nil {
		log.Fatal(err)
	}

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS sessions (id INTEGER PRIMARY KEY AUTOINCREMENT, token TEXT, username TEXT, created TIMESTAMP, expiry TIMESTAMP);
	CREATE TABLE IF NOT EXISTS playlists (id INTEGER PRIMARY KEY AUTOINCREMENT, enabled INTEGER, name TEXT, start_time TEXT);
	CREATE TABLE IF NOT EXISTS playlist_entries (id INTEGER PRIMARY KEY AUTOINCREMENT, playlist_id INTEGER, position INTEGER, filename TEXT, delay_seconds INTEGER, is_relative INTEGER, CONSTRAINT fk_playlists FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE);
	CREATE TABLE IF NOT EXISTS radios (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, token TEXT);
	CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT UNIQUE, password_hash TEXT, is_admin INTEGER);

	DELETE FROM sessions WHERE expiry < CURRENT_TIMESTAMP;
	`
	_, err = db.sqldb.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}
}

func (d *Database) CloseDatabase() {
	d.sqldb.Close()
}

func (d *Database) InsertSession(user string, token string, expiry time.Time) {
	_, err := d.sqldb.Exec("INSERT INTO sessions (token, username, created, expiry) values (?, ?, CURRENT_TIMESTAMP, ?)", token, user, expiry)
	if err != nil {
		log.Fatal(err)
	}
}

func (d *Database) GetUserNameForSession(token string) (string, error) {
	var username string
	err := d.sqldb.QueryRow("SELECT username FROM sessions WHERE token = ? AND expiry > CURRENT_TIMESTAMP", token).Scan(&username)
	if err != nil {
		return "", errors.New("no matching token")
	}
	return username, nil
}

func (d *Database) GetUser(username string) (User, error) {
	var user User
	err := d.sqldb.QueryRow("SELECT id, username, password_hash, is_admin FROM users WHERE username = ?", username).Scan(&user.Id, &user.Username, &user.PasswordHash, &user.IsAdmin)
	if err != nil {
		return User{}, errors.New("no user with that username")
	}
	return user, nil
}

func (d *Database) GetUserById(id int) (User, error) {
	var user User
	err := d.sqldb.QueryRow("SELECT id, username, password_hash, is_admin FROM users WHERE id = ?", id).Scan(&user.Id, &user.Username, &user.PasswordHash, &user.IsAdmin)
	if err != nil {
		return User{}, errors.New("no user with that id")
	}
	return user, nil
}

func (d *Database) GetUsers() []User {
	ret := make([]User, 0)
	rows, err := d.sqldb.Query("SELECT id, username, password_hash, is_admin FROM users ORDER BY username ASC")
	if err != nil {
		return ret
	}
	defer rows.Close()
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.Id, &u.Username, &u.PasswordHash, &u.IsAdmin); err != nil {
			return ret
		}
		ret = append(ret, u)
	}
	return ret
}

func (d *Database) SetUserPassword(username string, passwordHash string) {
	d.sqldb.Exec("UPDATE users SET password_hash = ? WHERE username = ?", passwordHash, username)
}

func (d *Database) ClearOtherSessions(username string, token string) {
	d.sqldb.Exec("DELETE FROM sessions WHERE username = ? AND token != ?", username, token)
}

func (d *Database) ClearSession(username string, token string) {
	d.sqldb.Exec("DELETE FROM sessions WHERE username = ? AND token = ?", username, token)
}

func (d *Database) SetUserIsAdmin(username string, isAdmin bool) {
	d.sqldb.Exec("UPDATE users SET is_admin = ? WHERE username = ?", isAdmin, username)
}

func (d *Database) CreateUser(user User) error {
	_, err := d.sqldb.Exec("INSERT INTO users (username, password_hash, is_admin) values (?, ?, ?)", user.Username, user.PasswordHash, user.IsAdmin)
	return err
}

func (d *Database) DeleteUser(username string) error {
	_, err := d.sqldb.Exec("DELETE FROM users WHERE username = ?", username)
	return err
}

func (d *Database) CreatePlaylist(playlist Playlist) int {
	var id int
	tx, _ := d.sqldb.Begin()
	_, err := tx.Exec("INSERT INTO playlists (enabled, name, start_time) values (?, ?, ?)", playlist.Enabled, playlist.Name, playlist.StartTime)
	if err != nil {
		log.Fatal(err)
	}
	err = tx.QueryRow("SELECT last_insert_rowid()").Scan(&id)
	if err != nil {
		log.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		log.Fatal(err)
	}
	return id
}

func (d *Database) DeletePlaylist(playlistId int) {
	d.sqldb.Exec("DELETE FROM playlists WHERE id = ?", playlistId)
}

func (d *Database) GetPlaylists() []Playlist {
	ret := make([]Playlist, 0)
	rows, err := d.sqldb.Query("SELECT id, enabled, name, start_time FROM playlists ORDER BY id ASC")
	if err != nil {
		return ret
	}
	defer rows.Close()
	for rows.Next() {
		var p Playlist
		if err := rows.Scan(&p.Id, &p.Enabled, &p.Name, &p.StartTime); err != nil {
			return ret
		}
		ret = append(ret, p)
	}
	return ret
}

func (d *Database) GetPlaylist(playlistId int) (Playlist, error) {
	var p Playlist
	err := d.sqldb.QueryRow("SELECT id, enabled, name, start_time FROM playlists WHERE id = ?", playlistId).Scan(&p.Id, &p.Enabled, &p.Name, &p.StartTime)
	if err != nil {
		return p, err
	}
	return p, nil
}

func (d *Database) UpdatePlaylist(playlist Playlist) {
	d.sqldb.Exec("UPDATE playlists SET enabled = ?, name = ?, start_time = ? WHERE id = ?", playlist.Enabled, playlist.Name, playlist.StartTime, playlist.Id)
}

func (d *Database) SetEntriesForPlaylist(entries []PlaylistEntry, playlistId int) {
	tx, _ := d.sqldb.Begin()
	_, err := tx.Exec("DELETE FROM playlist_entries WHERE playlist_id = ?", playlistId)
	for _, e := range entries {
		_, err = tx.Exec("INSERT INTO playlist_entries (playlist_id, position, filename, delay_seconds, is_relative) values (?, ?, ?, ?, ?)", playlistId, e.Position, e.Filename, e.DelaySeconds, e.IsRelative)
		if err != nil {
			log.Fatal(err)
		}
	}
	tx.Commit() // ignore errors
}

func (d *Database) GetEntriesForPlaylist(playlistId int) []PlaylistEntry {
	ret := make([]PlaylistEntry, 0)
	rows, err := d.sqldb.Query("SELECT id, position, filename, delay_seconds, is_relative FROM playlist_entries WHERE playlist_id = ? ORDER by position ASC", playlistId)
	if err != nil {
		return ret
	}
	defer rows.Close()
	for rows.Next() {
		var entry PlaylistEntry
		if err := rows.Scan(&entry.Id, &entry.Position, &entry.Filename, &entry.DelaySeconds, &entry.IsRelative); err != nil {
			return ret
		}
		ret = append(ret, entry)
	}
	return ret
}

func (d *Database) GetRadio(radioId int) (Radio, error) {
	var r Radio
	err := d.sqldb.QueryRow("SELECT id, name, token FROM radios WHERE id = ?", radioId).Scan(&r.Id, &r.Name, &r.Token)
	if err != nil {
		return r, err
	}
	return r, nil
}

func (d *Database) GetRadioByToken(token string) (Radio, error) {
	var r Radio
	err := d.sqldb.QueryRow("SELECT id, name, token FROM radios WHERE token = ?", token).Scan(&r.Id, &r.Name, &r.Token)
	if err != nil {
		return r, err
	}
	return r, nil
}

func (d *Database) GetRadios() []Radio {
	ret := make([]Radio, 0)
	rows, err := d.sqldb.Query("SELECT id, name, token FROM radios ORDER BY id ASC")
	if err != nil {
		return ret
	}
	defer rows.Close()
	for rows.Next() {
		var r Radio
		if err := rows.Scan(&r.Id, &r.Name, &r.Token); err != nil {
			return ret
		}
		ret = append(ret, r)
	}
	return ret
}

func (d *Database) DeleteRadio(radioId int) {
	d.sqldb.Exec("DELETE FROM radios WHERE id = ?", radioId)
}

func (d *Database) CreateRadio(radio Radio) {
	d.sqldb.Exec("INSERT INTO radios (name, token) values (?, ?)", radio.Name, radio.Token)
}

func (d *Database) UpdateRadio(radio Radio) {
	d.sqldb.Exec("UPDATE radios SET name = ?, token = ? WHERE id = ?", radio.Name, radio.Token, radio.Id)
}
