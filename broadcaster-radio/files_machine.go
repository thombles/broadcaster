package main

import (
	"code.octet-stream.net/broadcaster/internal/protocol"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type FilesMachine struct {
	specs     []protocol.FileSpec
	cachePath string
	missing   []string
}

func NewFilesMachine(cachePath string) FilesMachine {
	if err := os.MkdirAll(cachePath, 0750); err != nil {
		log.Fatal(err)
	}
	return FilesMachine{
		cachePath: cachePath,
	}
}

func (m *FilesMachine) UpdateSpecs(specs []protocol.FileSpec) {
	m.specs = specs
	m.RefreshMissing()
}

func (m *FilesMachine) RefreshMissing() {
	// Delete any files in the cache dir who are not in the spec
	entries, err := os.ReadDir(m.cachePath)
	if err != nil {
		log.Fatal(err)
	}
	okay := make([]string, 0)
	for _, file := range entries {
		hash := ""
		for _, spec := range m.specs {
			if file.Name() == spec.Name {
				hash = spec.Hash
				break
			}
		}
		// if we have an extraneous file, delete it
		if hash == "" {
			log.Println("Deleting extraneous cached audio file:", file.Name())
			os.Remove(filepath.Join(m.cachePath, file.Name()))
			continue
		}
		// if the hash isn't right, delete it
		f, err := os.Open(filepath.Join(m.cachePath, file.Name()))
		if err != nil {
			log.Fatal(err)
		}
		hasher := sha256.New()
		io.Copy(hasher, f)
		if hex.EncodeToString(hasher.Sum(nil)) != hash {
			log.Println("Deleting cached audio file with incorrect hash:", file.Name())
			os.Remove(filepath.Join(m.cachePath, file.Name()))
		} else {
			okay = append(okay, file.Name())
		}
	}
	m.missing = nil
	for _, spec := range m.specs {
		missing := true
		for _, file := range okay {
			if spec.Name == file {
				missing = false
			}
		}
		if missing {
			m.missing = append(m.missing, spec.Name)
		}
	}
	if len(m.missing) > 1 {
		log.Println(len(m.missing), "missing files")
	} else if len(m.missing) == 1 {
		log.Println("1 missing file")
	} else {
		log.Println("All files are in sync with server")
	}
	statusCollector.FilesInSync <- len(m.missing) == 0
}

func (m *FilesMachine) IsCacheComplete() bool {
	return len(m.missing) == 0
}

func (m *FilesMachine) NextFile() string {
	next, remainder := m.missing[0], m.missing[1:]
	m.missing = remainder
	return next
}

func (m *FilesMachine) DownloadSingle(filename string, downloadResult chan<- error) {
	log.Println("Downloading", filename)
	out, err := os.Create(filepath.Join(m.cachePath, filename))
	if err != nil {
		downloadResult <- err
		return
	}
	defer out.Close()
	resp, err := http.Get(config.ServerURL + "/file-downloads/" + filename)
	if err != nil {
		downloadResult <- err
		return
	}
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	downloadResult <- err
}
