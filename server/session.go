package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

func generateSession() string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func currentUser(w http.ResponseWriter, r *http.Request) (User, error) {
	// todo: check if user actually exists and is allowed to log in
	cookie, e := r.Cookie("broadcast_session")
	if e != nil {
		return User{}, e
	}

	username, e := db.GetUserForSession(cookie.Value)
	if e != nil {
		return User{}, e
	}
	return User{username: username}, nil
}

func createSessionCookie(w http.ResponseWriter) {
	sess := generateSession()
	log.Println("Generated a random session", sess)
	expiration := time.Now().Add(365 * 24 * time.Hour)
	cookie := http.Cookie{Name: "broadcast_session", Value: sess, Expires: expiration, SameSite: http.SameSiteLaxMode}
	db.InsertSession("admin", sess, expiration)
	http.SetCookie(w, &cookie)
}

func clearSessionCookie(w http.ResponseWriter) {
	c := &http.Cookie{
		Name:     "broadcast_session",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, c)
}
