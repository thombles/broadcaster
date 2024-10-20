package main

import (
	"flag"
	"golang.org/x/net/websocket"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const formatString = "2006-01-02T15:04"

var config ServerConfig = NewServerConfig()

func main() {
	configFlag := flag.String("c", "", "path to configuration file")
	// TODO: support this
	//generateFlag := flag.String("g", "", "create a template config file with specified name then exit")
	flag.Parse()

	if *configFlag == "" {
		log.Fatal("must specify a configuration file with -c")
	}
	config.LoadFromFile(*configFlag)

	log.Println("Hello, World! Woo broadcast time")
	InitDatabase()
	defer db.CloseDatabase()

	InitCommandRouter()
	InitPlaylists()
	InitAudioFiles(config.AudioFilesPath)
	InitServerStatus()

	http.HandleFunc("/", homePage)
	http.HandleFunc("/login", logInPage)
	http.HandleFunc("/logout", logOutPage)
	http.HandleFunc("/secret", secretPage)
	http.HandleFunc("/stop", stopPage)

	http.HandleFunc("/playlist/", playlistSection)
	http.HandleFunc("/file/", fileSection)
	http.HandleFunc("/radio/", radioSection)

	http.Handle("/radiosync", websocket.Handler(RadioSync))
	http.Handle("/websync", websocket.Handler(WebSync))
	http.Handle("/audio-files/", http.StripPrefix("/audio-files/", http.FileServer(http.Dir(config.AudioFilesPath))))

	err := http.ListenAndServe(config.BindAddress+":"+strconv.Itoa(config.Port), nil)
	if err != nil {
		log.Fatal(err)
	}
}

type HomeData struct {
	LoggedIn bool
	Username string
}

func homePage(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	data := HomeData{
		LoggedIn: true,
		Username: "Bob",
	}
	tmpl.Execute(w, data)
}

func secretPage(w http.ResponseWriter, r *http.Request) {
	user, err := currentUser(w, r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	data := HomeData{
		LoggedIn: true,
		Username: user.username + ", you are special",
	}
	tmpl.Execute(w, data)
}

type LogInData struct {
	Error string
}

func logInPage(w http.ResponseWriter, r *http.Request) {
	log.Println("Log in page!")
	r.ParseForm()
	username := r.Form["username"]
	password := r.Form["password"]
	err := ""
	if username != nil {
		log.Println("Looks like we have a username", username[0])
		if username[0] == "admin" && password[0] == "test" {
			createSessionCookie(w)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		} else {
			err = "Incorrect login"
		}
	}

	data := LogInData{
		Error: err,
	}

	tmpl := template.Must(template.ParseFiles("templates/login.html"))
	tmpl.Execute(w, data)
}

func playlistSection(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.URL.Path, "/")
	if len(path) != 3 {
		http.NotFound(w, r)
		return
	}
	if path[2] == "new" {
		editPlaylistPage(w, r, 0)
	} else if path[2] == "submit" && r.Method == "POST" {
		submitPlaylist(w, r)
	} else if path[2] == "delete" && r.Method == "POST" {
		deletePlaylist(w, r)
	} else if path[2] == "" {
		playlistsPage(w, r)
	} else {
		id, err := strconv.Atoi(path[2])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		editPlaylistPage(w, r, id)
	}
}

func fileSection(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.URL.Path, "/")
	if len(path) != 3 {
		http.NotFound(w, r)
		return
	}
	if path[2] == "upload" {
		uploadFile(w, r)
	} else if path[2] == "delete" && r.Method == "POST" {
		deleteFile(w, r)
	} else if path[2] == "" {
		filesPage(w, r)
	} else {
		http.NotFound(w, r)
		return
	}
}

func radioSection(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.URL.Path, "/")
	if len(path) != 3 {
		http.NotFound(w, r)
		return
	}
	if path[2] == "new" {
		editRadioPage(w, r, 0)
	} else if path[2] == "submit" && r.Method == "POST" {
		submitRadio(w, r)
	} else if path[2] == "delete" && r.Method == "POST" {
		deleteRadio(w, r)
	} else if path[2] == "" {
		radiosPage(w, r)
	} else {
		id, err := strconv.Atoi(path[2])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		editRadioPage(w, r, id)
	}
}

type PlaylistsPageData struct {
	Playlists []Playlist
}

func playlistsPage(w http.ResponseWriter, _ *http.Request) {
	data := PlaylistsPageData{
		Playlists: db.GetPlaylists(),
	}
	tmpl := template.Must(template.ParseFiles("templates/playlists.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
}

type RadiosPageData struct {
	Radios []Radio
}

func radiosPage(w http.ResponseWriter, _ *http.Request) {
	data := RadiosPageData{
		Radios: db.GetRadios(),
	}
	tmpl := template.Must(template.ParseFiles("templates/radios.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
}

type EditPlaylistPageData struct {
	Playlist Playlist
	Entries  []PlaylistEntry
	Files    []string
}

func editPlaylistPage(w http.ResponseWriter, r *http.Request, id int) {
	var data EditPlaylistPageData
	for _, f := range files.Files() {
		data.Files = append(data.Files, f.Name)
	}
	if id == 0 {
		data.Playlist.Enabled = true
		data.Playlist.Name = "New Playlist"
		data.Playlist.StartTime = time.Now().Format(formatString)
		data.Entries = append(data.Entries, PlaylistEntry{})
	} else {
		playlist, err := db.GetPlaylist(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data.Playlist = playlist
		data.Entries = db.GetEntriesForPlaylist(id)
	}
	tmpl := template.Must(template.ParseFiles("templates/playlist.html"))
	tmpl.Execute(w, data)
}

func submitPlaylist(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		var p Playlist
		id, err := strconv.Atoi(r.Form.Get("playlistId"))
		if err != nil {
			return
		}
		_, err = time.Parse(formatString, r.Form.Get("playlistStartTime"))
		if err != nil {
			return
		}
		p.Id = id
		p.Enabled = r.Form.Get("playlistEnabled") == "1"
		p.Name = r.Form.Get("playlistName")
		p.StartTime = r.Form.Get("playlistStartTime")

		delays := r.Form["delaySeconds"]
		filenames := r.Form["filename"]
		isRelatives := r.Form["isRelative"]

		entries := make([]PlaylistEntry, 0)
		for i := range delays {
			var e PlaylistEntry
			delay, err := strconv.Atoi(delays[i])
			if err != nil {
				return
			}
			e.DelaySeconds = delay
			e.Position = i
			e.IsRelative = isRelatives[i] == "1"
			e.Filename = filenames[i]
			entries = append(entries, e)
		}
		cleanedEntries := make([]PlaylistEntry, 0)
		for _, e := range entries {
			if e.DelaySeconds != 0 || e.Filename != "" {
				cleanedEntries = append(cleanedEntries, e)
			}
		}

		if id != 0 {
			db.UpdatePlaylist(p)
		} else {
			id = db.CreatePlaylist(p)
		}
		db.SetEntriesForPlaylist(cleanedEntries, id)
		// Notify connected radios
		playlists.NotifyChanges()
	}
	http.Redirect(w, r, "/playlist/", http.StatusFound)
}

func deletePlaylist(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		id, err := strconv.Atoi(r.Form.Get("playlistId"))
		if err != nil {
			return
		}
		db.DeletePlaylist(id)
		playlists.NotifyChanges()
	}
	http.Redirect(w, r, "/playlist/", http.StatusFound)
}

type EditRadioPageData struct {
	Radio Radio
}

func editRadioPage(w http.ResponseWriter, r *http.Request, id int) {
	var data EditRadioPageData
	if id == 0 {
		data.Radio.Name = "New Radio"
		data.Radio.Token = generateSession()
	} else {
		radio, err := db.GetRadio(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data.Radio = radio
	}
	tmpl := template.Must(template.ParseFiles("templates/radio.html"))
	tmpl.Execute(w, data)
}

func submitRadio(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		var radio Radio
		id, err := strconv.Atoi(r.Form.Get("radioId"))
		if err != nil {
			return
		}
		radio.Id = id
		radio.Name = r.Form.Get("radioName")
		radio.Token = r.Form.Get("radioToken")
		if id != 0 {
			db.UpdateRadio(radio)
		} else {
			db.CreateRadio(radio)
		}
	}
	http.Redirect(w, r, "/radio/", http.StatusFound)
}

func deleteRadio(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		id, err := strconv.Atoi(r.Form.Get("radioId"))
		if err != nil {
			return
		}
		db.DeleteRadio(id)
	}
	http.Redirect(w, r, "/radio/", http.StatusFound)
}

type FilesPageData struct {
	Files []FileSpec
}

func filesPage(w http.ResponseWriter, _ *http.Request) {
	data := FilesPageData{
		Files: files.Files(),
	}
	log.Println("file page data", data)
	tmpl := template.Must(template.ParseFiles("templates/files.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
}

func deleteFile(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		filename := r.Form.Get("filename")
		if filename == "" {
			return
		}
		files.Delete(filename)
	}
	http.Redirect(w, r, "/file/", http.StatusFound)
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(100 << 20)
	file, handler, err := r.FormFile("file")
	if err == nil {
		path := filepath.Join(files.Path(), filepath.Base(handler.Filename))
		f, _ := os.Create(path)
		defer f.Close()
		io.Copy(f, file)
		log.Println("uploaded file to", path)
		files.Refresh()
	}
	http.Redirect(w, r, "/file/", http.StatusFound)
}

func logOutPage(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	tmpl := template.Must(template.ParseFiles("templates/logout.html"))
	tmpl.Execute(w, nil)
}

func stopPage(w http.ResponseWriter, r *http.Request) {
	_, err := currentUser(w, r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	r.ParseForm()
	radioId, err := strconv.Atoi(r.Form.Get("radioId"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	commandRouter.Stop(radioId)
	http.Redirect(w, r, "/", http.StatusFound)
}
