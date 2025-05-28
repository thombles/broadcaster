package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"code.octet-stream.net/broadcaster/internal/protocol"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/gopxl/beep/v2/wav"
	"golang.org/x/net/websocket"
)

const version = "v1.0.0"
const sampleRate = 44100

var config RadioConfig = NewRadioConfig()

func main() {
	configFlag := flag.String("c", "", "path to configuration file")
	versionFlag := flag.Bool("v", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("Broadcaster Radio", version)
		os.Exit(0)
	}
	if *configFlag == "" {
		log.Fatal("must specify a configuration file with -c")
	}

	log.Println("Broadcaster Radio", version, "starting up")
	config.LoadFromFile(*configFlag)
	statusCollector.Config <- config

	playbackSampleRate := beep.SampleRate(sampleRate)
	speaker.Init(playbackSampleRate, playbackSampleRate.N(time.Second/10))

	if config.PTTPin != -1 {
		InitRaspberryPiPTT(config.PTTPin, config.GpioDevice)
	}
	if config.COSPin != -1 {
		InitRaspberryPiCOS(config.COSPin, config.GpioDevice)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sig
		log.Println("Radio shutting down due to signal:", sig)
		// Make sure we always stop PTT when program ends
		ptt.DisengagePTT()
		os.Exit(0)
	}()

	log.Println("Config checks out, radio coming online")
	log.Println("Audio file cache:", config.CachePath)

	fileSpecChan := make(chan []protocol.FileSpec)
	go filesWorker(config.CachePath, fileSpecChan)

	stop := make(chan bool)
	playlistSpecChan := make(chan []protocol.PlaylistSpec)
	go playlistWorker(playlistSpecChan, stop)

	for {
		runWebsocket(fileSpecChan, playlistSpecChan, stop)
		log.Println("Websocket failed, retry in 30 seconds")
		time.Sleep(time.Second * time.Duration(30))
	}
}

func runWebsocket(fileSpecChan chan []protocol.FileSpec, playlistSpecChan chan []protocol.PlaylistSpec, stop chan bool) error {
	log.Println("Establishing websocket connection to:", config.WebsocketURL())
	ws, err := websocket.Dial(config.WebsocketURL(), "", config.ServerURL)
	if err != nil {
		return err
	}

	auth := protocol.AuthenticateMessage{
		T:     "authenticate",
		Token: config.Token,
	}
	msg, _ := json.Marshal(auth)

	if _, err := ws.Write(msg); err != nil {
		log.Fatal(err)
	}
	statusCollector.Websocket <- ws

	buf := make([]byte, 16384)
	badRead := false
	for {
		n, err := ws.Read(buf)
		if err != nil {
			log.Println("Lost websocket to server")
			return err
		}
		// Ignore any massively oversize messages
		if n == len(buf) {
			badRead = true
			continue
		} else if badRead {
			badRead = false
			continue
		}

		t, msg, err := protocol.ParseMessage(buf[:n])
		if err != nil {
			log.Println("Message parse error", err)
			return err
		}

		if t == protocol.FilesType {
			filesMsg := msg.(protocol.FilesMessage)
			fileSpecChan <- filesMsg.Files
		}

		if t == protocol.PlaylistsType {
			playlistsMsg := msg.(protocol.PlaylistsMessage)
			playlistSpecChan <- playlistsMsg.Playlists
		}

		if t == protocol.StopType {
			log.Println("Received stop transmission message from server")
			stop <- true
		}
	}
}

func filesWorker(cachePath string, ch chan []protocol.FileSpec) {
	machine := NewFilesMachine(cachePath)
	isDownloading := false
	downloadResult := make(chan error)
	var timer *time.Timer

	for {
		var timerCh <-chan time.Time = nil
		if timer != nil {
			timerCh = timer.C
		}
		doNext := false
		select {
		case specs := <-ch:
			log.Println("Received new file specs", specs)
			machine.UpdateSpecs(specs)
			doNext = true
			timer = nil
		case err := <-downloadResult:
			isDownloading = false
			machine.RefreshMissing()
			if err != nil {
				log.Println(err)
				if !machine.IsCacheComplete() {
					timer = time.NewTimer(30 * time.Second)
				}
			} else {
				if !machine.IsCacheComplete() {
					timer = time.NewTimer(10 * time.Millisecond)
				}
			}
		case <-timerCh:
			doNext = true
			timer = nil
		}

		if doNext && !isDownloading && !machine.IsCacheComplete() {
			next := machine.NextFile()
			isDownloading = true
			go machine.DownloadSingle(next, downloadResult)
		}
	}
}

func playlistWorker(ch <-chan []protocol.PlaylistSpec, stop <-chan bool) {
	var specs []protocol.PlaylistSpec
	isPlaying := false
	playbackFinished := make(chan error)
	cancel := make(chan bool)
	nextId := 0
	var timer *time.Timer

	for {
		var timerCh <-chan time.Time = nil
		if timer != nil {
			timerCh = timer.C
		}
		doNext := false
		select {
		case specs = <-ch:
			log.Println("Received new playlist specs", specs)
			doNext = true
		case <-playbackFinished:
			isPlaying = false
			doNext = true
			cancel = make(chan bool)
		case <-timerCh:
			timer = nil
			isPlaying = true
			for _, v := range specs {
				if v.Id == nextId {
					go playPlaylist(v, playbackFinished, cancel)
				}
			}
		case <-stop:
			if isPlaying {
				log.Println("Cancelling playlist in progress")
				cancel <- true
			}
		}

		if doNext && !isPlaying {
			timer = nil
			found := false
			loc, err := time.LoadLocation(config.TimeZone)
			if err != nil {
				log.Fatal(err)
			}
			var soonestTime time.Time
			for _, v := range specs {
				t, err := time.ParseInLocation(protocol.StartTimeFormat, v.StartTime, loc)
				if err != nil {
					log.Println("Error parsing start time", err)
					continue
				}
				if t.Before(time.Now()) {
					continue
				}
				if !found || t.Before(soonestTime) {
					soonestTime = t
					found = true
					nextId = v.Id
				}
			}
			if found {
				duration := soonestTime.Sub(time.Now())
				log.Println("Next playlist will be id", nextId, "in", duration.Seconds(), "seconds")
				timer = time.NewTimer(duration)
			} else {
				log.Println("No future playlists")
			}
		}
	}
}

func playPlaylist(playlist protocol.PlaylistSpec, playbackFinished chan<- error, cancel <-chan bool) {
	startTime := time.Now()
	log.Println("Beginning playback of playlist", playlist.Name)
entries:
	for _, p := range playlist.Entries {
		// delay
		var duration time.Duration
		if p.IsRelative {
			duration = time.Second * time.Duration(p.DelaySeconds)
		} else {
			duration = time.Until(startTime.Add(time.Second * time.Duration(p.DelaySeconds)))
		}
		statusCollector.PlaylistBeginDelay <- BeginDelayStatus{
			Playlist: playlist.Name,
			Seconds:  int(duration.Seconds()),
			Filename: p.Filename,
		}
		select {
		case <-time.After(duration):
		case <-cancel:
			log.Println("Cancelling pre-play delay")
			break entries
		}

		statusCollector.PlaylistBeginWaitForChannel <- BeginWaitForChannelStatus{
			Playlist: playlist.Name,
			Filename: p.Filename,
		}
		cos.WaitForChannelClear()

		// then play
		statusCollector.PlaylistBeginPlayback <- BeginPlaybackStatus{
			Playlist: playlist.Name,
			Filename: p.Filename,
		}
		f, err := os.Open(filepath.Join(config.CachePath, p.Filename))
		if err != nil {
			log.Println("Couldn't open file for playlist", p.Filename)
			continue
		}
		log.Println("Playing file", p.Filename)
		l := strings.ToLower(p.Filename)
		var streamer beep.StreamSeekCloser
		var format beep.Format
		if strings.HasSuffix(l, ".mp3") {
			streamer, format, err = mp3.Decode(f)
		} else if strings.HasSuffix(l, ".wav") {
			streamer, format, err = wav.Decode(f)
		} else {
			log.Println("Unrecognised file extension (.wav and .mp3 supported), moving on")
		}
		if err != nil {
			log.Println("Could not decode media file", err)
			continue
		}
		defer streamer.Close()

		done := make(chan bool)
		log.Println("PTT on for playback")
		ptt.EngagePTT()

		if format.SampleRate != sampleRate {
			log.Println("Configuring resampler for audio provided at sample rate", format.SampleRate)
			resampled := beep.Resample(4, format.SampleRate, sampleRate, streamer)
			log.Println("Playing resampled audio")
			speaker.Play(beep.Seq(resampled, beep.Callback(func() {
				done <- true
			})))
		} else {
			log.Println("Playing audio at native sample rate")
			speaker.Play(beep.Seq(streamer, beep.Callback(func() {
				done <- true
			})))
		}

		select {
		case <-done:
			log.Println("Audio playback complete")
		case <-cancel:
			log.Println("Disengaging PTT and aborting playlist playback")
			ptt.DisengagePTT()
			break entries
		}
		log.Println("PTT off since audio file has finished")
		ptt.DisengagePTT()
	}
	log.Println("Playlist finished", playlist.Name)
	statusCollector.PlaylistBeginIdle <- true
	playbackFinished <- nil
}
