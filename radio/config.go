package main

import (
	"errors"
	"log"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type RadioConfig struct {
	GpioDevice string
	PTTPin     int
	COSPin     int
	ServerURL  string
	Token      string
	CachePath  string
	TimeZone   string
}

func NewRadioConfig() RadioConfig {
	return RadioConfig{
		GpioDevice: "gpiochip0",
		PTTPin:     -1,
		COSPin:     -1,
		ServerURL:  "",
		Token:      "",
		CachePath:  "",
		TimeZone:   "Local",
	}
}

func (c *RadioConfig) LoadFromFile(path string) {
	_, err := toml.DecodeFile(path, &c)
	if err != nil {
		log.Fatal("could not read config file for reading at path:", path, err)
	}
	err = c.Validate()
	if err != nil {
		log.Fatal(err)
	}
	c.ApplyDefaults()
}

func (c *RadioConfig) Validate() error {
	if c.ServerURL == "" {
		return errors.New("ServerURL must be provided in the configuration")
	}
	if c.Token == "" {
		return errors.New("Token must be provided in the configuration")
	}
	return nil
}

func (c *RadioConfig) ApplyDefaults() {
	if c.CachePath == "" {
		dir, err := os.MkdirTemp("", "broadcast")
		if err != nil {
			log.Fatal(err)
		}
		c.CachePath = dir
	}
}

func (c *RadioConfig) WebsocketURL() string {
	addr := strings.Replace(c.ServerURL, "https://", "wss://", -1)
	addr = strings.Replace(addr, "http://", "ws://", -1)
	return addr + "/radio-ws"
}
