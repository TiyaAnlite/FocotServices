package main

import (
	"github.com/FishZe/go-bili-chat/v2/events"
	"sync/atomic"
	"time"
)

// BLiveEventHandlerWrapper wrap handler to chan
type BLiveEventHandlerWrapper struct {
	Command   string `json:"cmd"`
	EventChan chan<- *BLiveEventHandlerMsg
	Counter   *atomic.Uint32
}

func (h *BLiveEventHandlerWrapper) Cmd() string {
	return h.Command
}

func (h *BLiveEventHandlerWrapper) On(event *events.BLiveEvent) {
	if h.Counter != nil {
		h.Counter.Add(1)
	}
	h.EventChan <- &BLiveEventHandlerMsg{
		event:     event,
		startTime: time.Now(),
	}
}

type BLiveEventHandlerMsg struct {
	event       *events.BLiveEvent
	startTime   time.Time
	processTime time.Time
}
