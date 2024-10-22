package main

import (
	gpio "github.com/warthog618/go-gpiocdev"
	"github.com/warthog618/go-gpiocdev/device/rpi"
	"log"
	"strconv"
)

type PTT interface {
	EngagePTT()
	DisengagePTT()
}

type COS interface {
	WaitForChannelClear()
	COSValue() bool
}

var ptt PTT = &DefaultPTT{}
var cos COS = &DefaultCOS{}

type PiPTT struct {
	pttLine *gpio.Line
}

type PiCOS struct {
	cosLine   *gpio.Line
	clearWait chan bool
}

func InitRaspberryPiPTT(pttNum int, chipName string) {
	pttPin, err := rpi.Pin("GPIO" + strconv.Itoa(pttNum))
	if err != nil {
		log.Fatal("invalid PTT pin configured", ptt)
	}
	pttLine, err := gpio.RequestLine(chipName, pttPin, gpio.AsOutput(0))
	if err != nil {
		log.Fatal("unable to open requested pin for PTT GPIO:", ptt, ". Are you running as root?")
	}
	ptt = &PiPTT{
		pttLine: pttLine,
	}
}

func InitRaspberryPiCOS(cosNum int, chipName string) {
	var piCOS PiCOS
	piCOS.clearWait = make(chan bool)
	cosPin, err := rpi.Pin("GPIO" + strconv.Itoa(cosNum))
	if err != nil {
		log.Fatal("invalid COS Pin configured", cos)
	}
	cosHandler := func(event gpio.LineEvent) {
		if event.Type == gpio.LineEventFallingEdge {
			log.Println("COS: channel clear")
			close(piCOS.clearWait)
			piCOS.clearWait = make(chan bool)
			statusCollector.COS <- false
		}
		if event.Type == gpio.LineEventRisingEdge {
			log.Println("COS: channel in use")
			statusCollector.COS <- true
		}
	}
	cosLine, err := gpio.RequestLine(chipName, cosPin, gpio.AsInput, gpio.WithBothEdges, gpio.WithEventHandler(cosHandler))
	if err != nil {
		log.Fatal("unable to open requested pin for COS GPIO:", cos, ". Are you running as root?")
	}
	piCOS.cosLine = cosLine
	cos = &piCOS
}

func (g *PiCOS) COSValue() bool {
	val, err := g.cosLine.Value()
	if err != nil {
		log.Fatal("Unable to read COS value")
	}
	return val != 0
}

func (g *PiCOS) WaitForChannelClear() {
	ch := g.clearWait
	val, err := g.cosLine.Value()
	if err != nil || val == 0 {
		return
	}
	// wait for close
	<-ch
}

func (g *PiPTT) EngagePTT() {
	log.Println("PTT: on")
	g.pttLine.SetValue(1)
	statusCollector.PTT <- true
}

func (g *PiPTT) DisengagePTT() {
	log.Println("PTT: off")
	g.pttLine.SetValue(0)
	statusCollector.PTT <- false
}

type DefaultPTT struct {
}

func (g *DefaultPTT) EngagePTT() {
	statusCollector.PTT <- true
}

func (g *DefaultPTT) DisengagePTT() {
	statusCollector.PTT <- false
}

type DefaultCOS struct {
}

func (g *DefaultCOS) WaitForChannelClear() {
	log.Println("Assuming channel is clear since COS GPIO is not configured")
}

func (g *DefaultCOS) COSValue() bool {
	return false
}
