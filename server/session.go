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

func currentUser(_ http.ResponseWriter, r *http.Request) (User, error) {
	cookie, e := r.Cookie("broadcast_session")
	if e != nil {
		return User{}, e
	}

	return users.GetUserForSession(cookie.Value)
}

func createSessionCookie(w http.ResponseWriter, username string) {
	sess := generateSession()
	expiration := time.Now().Add(365 * 24 * time.Hour)
	cookie := http.Cookie{Name: "broadcast_session", Value: sess, Expires: expiration, SameSite: http.SameSiteLaxMode}
	db.InsertSession(username, sess, expiration)
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
