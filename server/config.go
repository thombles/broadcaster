package main

import (
	"errors"
	"log"

	"github.com/BurntSushi/toml"
)

type ServerConfig struct {
	BindAddress    string
	Port           int
	SqliteDB       string
	AudioFilesPath string
}

func NewServerConfig() ServerConfig {
	return ServerConfig{
		BindAddress:    "0.0.0.0",
		Port:           55134,
		SqliteDB:       "",
		AudioFilesPath: "",
	}
}

func (c *ServerConfig) LoadFromFile(path string) {
	_, err := toml.DecodeFile(path, &c)
	if err != nil {
		log.Fatal("could not read config file for reading at path:", path, err)
	}
	err = c.Validate()
	if err != nil {
		log.Fatal(err)
	}
}

func (c *ServerConfig) Validate() error {
	if c.SqliteDB == "" {
		return errors.New("Configuration must provide SqliteDB")
	}
	if c.AudioFilesPath == "" {
		return errors.New("Configuration must provide AudioFilesPath")
	}
	return nil
}
