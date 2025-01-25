package main

import (
	"github.com/FishZe/go-bili-chat/v2/events"
	"k8s.io/klog/v2"
	"sync/atomic"
	"time"
)

// BLiveEventHandlerWrapper wrap handler to chan
type BLiveEventHandlerWrapper struct {
	Command   string `json:"cmd"`
	EventChan chan<- *BLiveEventHandlerMsg
	Counter   *atomic.Int32
}

func (h *BLiveEventHandlerWrapper) Cmd() string {
	klog.V(5).Infof("BLiveEventHandlerWrapper Hook: cmd -> %s", h.Command)
	return h.Command
}

func (h *BLiveEventHandlerWrapper) On(event *events.BLiveEvent) {
	klog.V(5).Infof("New BliveEvent: %s", event.Cmd)
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
