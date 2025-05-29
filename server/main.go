package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"code.octet-stream.net/broadcaster/internal/protocol"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/websocket"
)

const version = "v1.2.0"

//go:embed templates/*
var content embed.FS

//var content = os.DirFS("../broadcaster-server/")

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
	http.Handle("/file-downloads/", applyDisposition(http.StripPrefix("/file-downloads/", http.FileServer(http.Dir(config.AudioFilesPath)))))

	// Authenticated routes

	http.Handle("/", requireUser(homePage))
	http.Handle("/logout", requireUser(logOutPage))
	http.Handle("/change-password", requireUser(changePasswordPage))

	http.Handle("/playlists/", requireUser(playlistSection))
	http.Handle("/files/", requireUser(fileSection))
	http.Handle("/radios/", requireUser(radioSection))

	http.Handle("/stop", requireUser(stopPage))

	// Admin routes

	http.Handle("/users/", requireAdmin(userSection))

	// Websocket routes, which perform their own auth

	http.Handle("/radio-ws", websocket.Handler(RadioSync))
	http.Handle("/web-ws", websocket.Handler(WebSync))

	err := http.ListenAndServe(config.BindAddress+":"+strconv.Itoa(config.Port), nil)
	if err != nil {
		log.Fatal(err)
	}
}

type DispositionMiddleware struct {
	handler http.Handler
}

func (m DispositionMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("path", r.URL.Path)
	if r.URL.Path != "/file-downloads/" {
		w.Header().Add("Content-Disposition", "attachment")
	}
	m.handler.ServeHTTP(w, r)
}

func applyDisposition(handler http.Handler) DispositionMiddleware {
	return DispositionMiddleware{
		handler: handler,
	}
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, User)

type AuthMiddleware struct {
	handler     authenticatedHandler
	mustBeAdmin bool
}

func (m AuthMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, err := currentUser(w, r)
	if err != nil || (m.mustBeAdmin && !user.IsAdmin) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	m.handler(w, r, user)
}

func requireUser(handler authenticatedHandler) AuthMiddleware {
	return AuthMiddleware{
		handler:     handler,
		mustBeAdmin: false,
	}
}

func requireAdmin(handler authenticatedHandler) AuthMiddleware {
	return AuthMiddleware{
		handler:     handler,
		mustBeAdmin: true,
	}
}

type HeaderData struct {
	SelectedMenu string
	User         User
	Version      string
}

func renderHeader(w http.ResponseWriter, selectedMenu string, user User) {
	tmpl := template.Must(template.ParseFS(content, "templates/header.html"))
	data := HeaderData{
		SelectedMenu: selectedMenu,
		User:         user,
		Version:      version,
	}
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
}

func renderFooter(w http.ResponseWriter) {
	tmpl := template.Must(template.ParseFS(content, "templates/footer.html"))
	err := tmpl.Execute(w, nil)
	if err != nil {
		log.Fatal(err)
	}
}

type HomeData struct {
	LoggedIn bool
	Username string
}

func homePage(w http.ResponseWriter, r *http.Request, user User) {
	renderHeader(w, "status", user)
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
	renderHeader(w, "", User{})
	tmpl := template.Must(template.ParseFS(content, "templates/login.html"))
	tmpl.Execute(w, data)
	renderFooter(w)
}

func playlistSection(w http.ResponseWriter, r *http.Request, user User) {
	path := strings.Split(r.URL.Path, "/")
	if len(path) != 3 {
		http.NotFound(w, r)
		return
	}
	if path[2] == "new" {
		editPlaylistPage(w, r, 0, user)
	} else if path[2] == "submit" && r.Method == "POST" {
		submitPlaylist(w, r)
	} else if path[2] == "delete" && r.Method == "POST" {
		deletePlaylist(w, r)
	} else if path[2] == "" {
		playlistsPage(w, r, user)
	} else {
		id, err := strconv.Atoi(path[2])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		editPlaylistPage(w, r, id, user)
	}
}

func fileSection(w http.ResponseWriter, r *http.Request, user User) {
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
		filesPage(w, r, user)
	} else {
		http.NotFound(w, r)
		return
	}
}

func radioSection(w http.ResponseWriter, r *http.Request, user User) {
	path := strings.Split(r.URL.Path, "/")
	if len(path) != 3 {
		http.NotFound(w, r)
		return
	}
	if path[2] == "new" {
		editRadioPage(w, r, 0, user)
	} else if path[2] == "submit" && r.Method == "POST" {
		submitRadio(w, r)
	} else if path[2] == "delete" && r.Method == "POST" {
		deleteRadio(w, r)
	} else if path[2] == "" {
		radiosPage(w, r, user)
	} else {
		id, err := strconv.Atoi(path[2])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		editRadioPage(w, r, id, user)
	}
}

func userSection(w http.ResponseWriter, r *http.Request, user User) {
	path := strings.Split(r.URL.Path, "/")
	if len(path) != 3 {
		http.NotFound(w, r)
		return
	}
	if path[2] == "new" {
		editUserPage(w, r, 0, user)
	} else if path[2] == "submit" && r.Method == "POST" {
		submitUser(w, r)
	} else if path[2] == "delete" && r.Method == "POST" {
		deleteUser(w, r)
	} else if path[2] == "reset-password" && r.Method == "POST" {
		resetUserPassword(w, r)
	} else if path[2] == "" {
		usersPage(w, r, user)
	} else {
		id, err := strconv.Atoi(path[2])
		if err != nil {
			http.NotFound(w, r)
			return
		}
		editUserPage(w, r, id, user)
	}
}

type EditUserPageData struct {
	User User
}

func editUserPage(w http.ResponseWriter, r *http.Request, id int, user User) {
	var data EditUserPageData
	if id != 0 {
		user, err := db.GetUserById(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data.User = user
	}
	renderHeader(w, "users", user)
	tmpl := template.Must(template.ParseFS(content, "templates/user.html"))
	tmpl.Execute(w, data)
	renderFooter(w)
}

func submitUser(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		id, err := strconv.Atoi(r.Form.Get("userId"))
		if err != nil {
			return
		}
		if id == 0 {
			newPassword := r.Form.Get("password")
			hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
			if err != nil {
				return
			}
			user := User{
				Id:           0,
				Username:     r.Form.Get("username"),
				IsAdmin:      r.Form.Get("isAdmin") == "1",
				PasswordHash: string(hashed),
			}
			db.CreateUser(user)
		} else {
			user, err := db.GetUserById(id)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			db.SetUserIsAdmin(user.Username, r.Form.Get("isAdmin") == "1")
		}
	}
	http.Redirect(w, r, "/users/", http.StatusFound)
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		id, err := strconv.Atoi(r.Form.Get("userId"))
		if err != nil {
			return
		}
		user, err := db.GetUserById(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		db.DeleteUser(user.Username)
	}
	http.Redirect(w, r, "/users/", http.StatusFound)
}

func resetUserPassword(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		id, err := strconv.Atoi(r.Form.Get("userId"))
		if err != nil {
			return
		}
		user, err := db.GetUserById(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		newPassword := r.Form.Get("newPassword")
		hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return
		}
		db.SetUserPassword(user.Username, string(hashed))
	}
	http.Redirect(w, r, "/users/", http.StatusFound)
}

type ChangePasswordPageData struct {
	Message  string
	ShowForm bool
}

func changePasswordPage(w http.ResponseWriter, r *http.Request, user User) {
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
				log.Println("Clearing other sessions for username", user.Username, "token", cookie.Value)
				db.ClearOtherSessions(user.Username, cookie.Value)
			}
		}
	} else {
		data.Message = ""
		data.ShowForm = true
	}
	renderHeader(w, "change-password", user)
	tmpl := template.Must(template.ParseFS(content, "templates/change_password.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
	renderFooter(w)
}

type UsersPageData struct {
	Users []User
}

func usersPage(w http.ResponseWriter, _ *http.Request, user User) {
	renderHeader(w, "users", user)
	data := UsersPageData{
		Users: db.GetUsers(),
	}
	tmpl := template.Must(template.ParseFS(content, "templates/users.html"))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Fatal(err)
	}
	renderFooter(w)
}

type PlaylistsPageData struct {
	Playlists []Playlist
}

func playlistsPage(w http.ResponseWriter, _ *http.Request, user User) {
	renderHeader(w, "playlists", user)
	data := PlaylistsPageData{
		Playlists: db.GetPlaylists(),
	}
	for i := range data.Playlists {
		data.Playlists[i].StartTime = strings.Replace(data.Playlists[i].StartTime, "T", " ", -1)
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

func radiosPage(w http.ResponseWriter, _ *http.Request, user User) {
	renderHeader(w, "radios", user)
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

func editPlaylistPage(w http.ResponseWriter, r *http.Request, id int, user User) {
	var data EditPlaylistPageData
	for _, f := range files.Files() {
		data.Files = append(data.Files, f.Name)
	}
	if id == 0 {
		data.Playlist.Enabled = true
		data.Playlist.Name = "New Playlist"
		data.Playlist.StartTime = time.Now().Format(protocol.StartTimeFormatSecs)
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
	renderHeader(w, "playlists", user)
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
		_, err = time.Parse(protocol.StartTimeFormatSecs, r.Form.Get("playlistStartTime"))
		if err != nil {
			_, err = time.Parse(protocol.StartTimeFormat, r.Form.Get("playlistStartTime"))
		}
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

func editRadioPage(w http.ResponseWriter, r *http.Request, id int, user User) {
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
	renderHeader(w, "radios", user)
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

func filesPage(w http.ResponseWriter, _ *http.Request, user User) {
	renderHeader(w, "files", user)
	data := FilesPageData{
		Files: files.Files(),
	}
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
		log.Println("Uploaded file to", path)
		files.Refresh()
	}
	http.Redirect(w, r, "/files/", http.StatusFound)
}

func logOutPage(w http.ResponseWriter, r *http.Request, user User) {
	cookie, err := r.Cookie("broadcast_session")
	if err == nil {
		db.ClearSession(user.Username, cookie.Value)
	}
	clearSessionCookie(w)
	renderHeader(w, "", user)
	tmpl := template.Must(template.ParseFS(content, "templates/logout.html"))
	tmpl.Execute(w, nil)
	renderFooter(w)
}

func stopPage(w http.ResponseWriter, r *http.Request, user User) {
	r.ParseForm()
	radioId, err := strconv.Atoi(r.Form.Get("radioId"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	commandRouter.Stop(radioId)
	http.Redirect(w, r, "/", http.StatusFound)
}
