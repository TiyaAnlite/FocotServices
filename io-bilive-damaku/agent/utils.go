package main

import "github.com/FishZe/go-bili-chat/v2/events"

// BLiveEventHandlerWrapper wrap handler to chan
type BLiveEventHandlerWrapper struct {
	Command   string `json:"cmd"`
	EventChan chan<- *events.BLiveEvent
}

func (h *BLiveEventHandlerWrapper) Cmd() string {
	return h.Command
}

func (h *BLiveEventHandlerWrapper) On(event *events.BLiveEvent) {
	h.EventChan <- event
}
