package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
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

const version = "v1.0.0"
const formatString = "2006-01-02T15:04"

//go:embed templates/*
var content embed.FS

var config ServerConfig = NewServerConfig()

func main() {
	configFlag := flag.String("c", "", "path to configuration file")
	addUserFlag := flag.Bool("a", false, "interactively add an admin user then exit")
	versionFlag := flag.Bool("v", false, "print version then exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("Broadcaster Server", version)
		os.Exit(0)
	}
	if *configFlag == "" {
		log.Fatal("must specify a configuration file with -c")
	}
	config.LoadFromFile(*configFlag)

	InitDatabase()
	defer db.CloseDatabase()

	if *addUserFlag {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("Enter new admin username:")
		if !scanner.Scan() {
			os.Exit(1)
		}
		username := scanner.Text()
		fmt.Println("Enter new admin password (will be printed in the clear):")
		if !scanner.Scan() {
			os.Exit(1)
		}
		password := scanner.Text()
		if username == "" || password == "" {
			fmt.Println("Both username and password must be specified")
			os.Exit(1)
		}
		if err := users.CreateUser(username, password, true); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	log.Println("Broadcaster Server", version, "starting up")
	InitCommandRouter()
	InitPlaylists()
	InitAudioFiles(config.AudioFilesPath)
	InitServerStatus()

	// Public routes

	http.HandleFunc("/login", logInPage)
	http.Handle("/file-downloads/", http.StripPrefix("/file-downloads/", http.FileServer(http.Dir(config.AudioFilesPath))))

	// Authenticated routes

	http.HandleFunc("/", homePage)
	http.HandleFunc("/logout", logOutPage)
	http.HandleFunc("/change-password", changePasswordPage)

	http.HandleFunc("/playlists/", playlistSection)
	http.HandleFunc("/files/", fileSection)
	http.HandleFunc("/radios/", radioSection)

	http.Handle("/radio-ws", websocket.Handler(RadioSync))
	http.Handle("/web-ws", websocket.Handler(WebSync))
	http.HandleFunc("/stop", stopPage)

	// Admin routes

	err := http.ListenAndServe(config.BindAddress+":"+strconv.Itoa(config.Port), nil)
	if err != nil {
		log.Fatal(err)
	}
}

type HeaderData struct {
	SelectedMenu string
}

func renderHeader(w http.ResponseWriter, selectedMenu string) {
	tmpl := template.Must(template.ParseFS(content, "templates/header.html"))
	data := HeaderData{
		SelectedMenu: selectedMenu,
	}
	tmpl.Execute(w, data)
}

func renderFooter(w http.ResponseWriter) {
	tmpl := template.Must(template.ParseFS(content, "templates/footer.html"))
	tmpl.Execute(w, nil)
}

type HomeData struct {
	LoggedIn bool
	Username string
}

func homePage(w http.ResponseWriter, r *http.Request) {
	renderHeader(w, "status")
	tmpl := template.Must(template.ParseFS(content, "templates/index.html"))
	data := HomeData{
		LoggedIn: true,
		Username: "Bob",
	}
	tmpl.Execute(w, data)
	renderFooter(w)
}

type LogInData struct {
	Error string
}

func logInPage(w http.ResponseWriter, r *http.Request) {
	log.Println("Log in page!")
	r.ParseForm()
	username := r.Form["username"]
	password := r.Form["password"]
	errText := ""
	if username != nil {
		user, err := users.Authenticate(username[0], password[0])
		if err != nil {
			errText = "Incorrect login"
		} else {
			createSessionCookie(w, user.Username)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}

	data := LogInData{
		Error: errText,
	}
	renderHeader(w, "")
	tmpl := template.Must(template.ParseFS(content, "templates/login.html"))
	tmpl.Execute(w, data)
	renderFooter(w)
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

type ChangePasswordPageData struct {
	Message  string
	ShowForm bool
}

func changePasswordPage(w http.ResponseWriter, r *http.Request) {
	user, err := currentUser(w, r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	var data ChangePasswordPageData
	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		oldPassword := r.Form.Get("oldPassword")
		newPassword := r.Form.Get("newPassword")
		err = users.UpdatePassword(user.Username, oldPassword, newPassword)
		if err != nil {
			data.Message = "Failed to change password: " + err.Error()
			data.ShowForm = true
		} else {
			data.Message = "Successfully changed password"
			data.ShowForm = false
			cookie, err := r.Cookie("broadcast_session")
			if err == nil {
				log.Println("clearing other sessions for username", user.Username, "token", cookie.Value)
				db.ClearOtherSessions(user.Username, cookie.Value)
			}
		}
	} else {
		data.Message = ""
		data.ShowForm = true
	}
	renderHeader(w, "change-password")
	tmpl := template.Must(template.ParseFS(content, "templates/change_password.html"))
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
	renderFooter(w)
}

type PlaylistsPageData struct {
	Playlists []Playlist
}

func playlistsPage(w http.ResponseWriter, _ *http.Request) {
	renderHeader(w, "playlists")
	data := PlaylistsPageData{
		Playlists: db.GetPlaylists(),
	}
	tmpl := template.Must(template.ParseFS(content, "templates/playlists.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
	renderFooter(w)
}

type RadiosPageData struct {
	Radios []Radio
}

func radiosPage(w http.ResponseWriter, _ *http.Request) {
	renderHeader(w, "radios")
	data := RadiosPageData{
		Radios: db.GetRadios(),
	}
	tmpl := template.Must(template.ParseFS(content, "templates/radios.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
	renderFooter(w)
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
	renderHeader(w, "radios")
	tmpl := template.Must(template.ParseFS(content, "templates/playlist.html"))
	tmpl.Execute(w, data)
	renderFooter(w)
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
	http.Redirect(w, r, "/playlists/", http.StatusFound)
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
	http.Redirect(w, r, "/playlists/", http.StatusFound)
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
	renderHeader(w, "radios")
	tmpl := template.Must(template.ParseFS(content, "templates/radio.html"))
	tmpl.Execute(w, data)
	renderFooter(w)
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
	http.Redirect(w, r, "/radios/", http.StatusFound)
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
	http.Redirect(w, r, "/radios/", http.StatusFound)
}

type FilesPageData struct {
	Files []FileSpec
}

func filesPage(w http.ResponseWriter, _ *http.Request) {
	renderHeader(w, "files")
	data := FilesPageData{
		Files: files.Files(),
	}
	log.Println("file page data", data)
	tmpl := template.Must(template.ParseFS(content, "templates/files.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
	renderFooter(w)
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
	http.Redirect(w, r, "/files/", http.StatusFound)
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
	http.Redirect(w, r, "/files/", http.StatusFound)
}

func logOutPage(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	renderHeader(w, "logout")
	tmpl := template.Must(template.ParseFS(content, "templates/logout.html"))
	tmpl.Execute(w, nil)
	renderFooter(w)
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
