package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type FileSpec struct {
	Name string
	Hash string
}

type AudioFiles struct {
	path       string
	list       []FileSpec
	changeWait chan bool
	filesMutex sync.Mutex
}

var files AudioFiles

func InitAudioFiles(path string) {
	files.changeWait = make(chan bool)
	files.path = path
	log.Println("initing audio files")
	files.Refresh()
	log.Println("done")
}

func (r *AudioFiles) Refresh() {
	entries, err := os.ReadDir(r.path)
	if err != nil {
		log.Println("couldn't read dir", r.path)
		return
	}
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	r.list = nil
	for _, file := range entries {
		f, err := os.Open(filepath.Join(r.path, file.Name()))
		if err != nil {
			log.Println("couldn't open", file.Name())
			return
		}
		hash := sha256.New()
		io.Copy(hash, f)
		r.list = append(r.list, FileSpec{Name: file.Name(), Hash: hex.EncodeToString(hash.Sum(nil))})
	}
	log.Println("Files updated", r.list)
	close(files.changeWait)
	files.changeWait = make(chan bool)
}

func (r *AudioFiles) Path() string {
	return r.path
}

func (r *AudioFiles) Files() []FileSpec {
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	return r.list
}

func (r *AudioFiles) Delete(filename string) {
	path := filepath.Join(r.path, filepath.Base(filename))
	if filepath.Clean(r.path) != filepath.Clean(path) {
		os.Remove(path)
		r.Refresh()
	}
}

func (r *AudioFiles) WatchForChanges() ([]FileSpec, chan bool) {
	r.filesMutex.Lock()
	defer r.filesMutex.Unlock()
	return r.list, r.changeWait
}
