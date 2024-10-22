package main

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
)

var users Users

type Users struct{}

func (u *Users) GetUserForSession(token string) (User, error) {
	username, err := db.GetUserNameForSession(token)
	if err != nil {
		return User{}, err
	}
	user, err := db.GetUser(username)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (u *Users) Authenticate(username string, clearPassword string) (User, error) {
	user, err := db.GetUser(username)
	if err != nil {
		return User{}, err
	}
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(clearPassword))
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (u *Users) CreateUser(username string, clearPassword string, isAdmin bool) error {
	if clearPassword == "" {
		return errors.New("password cannot be empty")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(clearPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return db.CreateUser(User{
		Id:           0,
		Username:     username,
		PasswordHash: string(hashed),
		IsAdmin:      isAdmin,
	})
}

func (u *Users) DeleteUser(username string) {
	db.DeleteUser(username)
}

func (u *Users) UpdatePassword(username string, oldClearPassword string, newClearPassword string) error {
	user, err := db.GetUser(username)
	if err != nil {
		return err
	}
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldClearPassword))
	if err != nil {
		return errors.New("old password is incorrect")
	}
	if newClearPassword == "" {
		return errors.New("password cannot be empty")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newClearPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	db.SetUserPassword(username, string(hashed))
	return nil
}

func (u *Users) UpdateIsAdmin(username string, isAdmin bool) {
	db.SetUserIsAdmin(username, isAdmin)
}

func (u *Users) Users() []User {
	return db.GetUsers()
}
