package main

import (
	"encoding/json"
	"fmt"
	"github.com/levigross/grequests"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

type DataSyncer struct {
	Recorders       map[string]string
	RefreshInterval int
	MaxRequestTime  int
	StreamName      string
	taskStatus      sync.Map // serverName:bool
	clients         sync.Map // serverName:Session
	connWarn        sync.Map // serverName:bool
	syncStarted     bool
	wg              *worker
}

func (syncer *DataSyncer) doSync(worker *worker) {
	if syncer.syncStarted {
		klog.Warningf("Syncer already started")
		return
	}
	syncer.wg = worker
	syncer.wg.Add(1)
	defer syncer.wg.Done()
	klog.Infof("Starting sync")
	// Init task status
	for serverName := range syncer.Recorders {
		syncer.taskStatus.Store(serverName, false)
		syncer.connWarn.Store(serverName, false)
	}
	ticker := time.NewTicker(time.Duration(int64(time.Second) * int64(syncer.RefreshInterval)))
	for {
		select {
		case <-ticker.C:
			syncer.taskStatus.Range(func(key, value any) bool {
				if value.(bool) {
					// Task padding
					return true
				} else {
					serverName := key.(string)
					url, ok := syncer.Recorders[serverName]
					if !ok {
						klog.Errorf("Unknown recorders: %s", serverName)
					}
					// klog.Infof("Sync %s", serverName)
					go syncer.doRequest(serverName, url)
				}
				return true
			})
		case <-syncer.wg.Ctx.Done():
			ticker.Stop()
			klog.Infof("Sync end.")
			syncer.syncStarted = false
			return
		}
	}
}

func (syncer *DataSyncer) doRequest(servername string, recorderUrl string) {
	syncer.wg.Add(1)
	defer syncer.wg.Done()
	syncer.taskStatus.Store(servername, true)
	defer syncer.taskStatus.Store(servername, false)
	client, ok := syncer.clients.Load(servername)
	if !ok {
		klog.Infof("[%s]New http client", servername)
		ro := &grequests.RequestOptions{
			RequestTimeout: time.Duration(int64(time.Second) * int64(syncer.MaxRequestTime)),
			DialKeepAlive:  time.Second * 30,
			Context:        syncer.wg.Ctx,
		}
		client = grequests.NewSession(ro)
		syncer.clients.Store(servername, client)
	}
	resp, err := client.(*grequests.Session).Get(fmt.Sprintf("http://%s/api/room", recorderUrl), nil)
	if err != nil {
		isWarned, _ := syncer.connWarn.Load(servername)
		if !isWarned.(bool) {
			klog.Errorf("[%s]On sync: %s", servername, err.Error())
			syncer.connWarn.Store(servername, true)
		}
		return
	}
	var rooms []BiliveRecorderRecord
	if err := resp.JSON(&rooms); err != nil {
		klog.Errorf("[%s]On parse rooms: %s", servername, err.Error())
		return
	}
	isWarned, _ := syncer.connWarn.Load(servername)
	if isWarned.(bool) {
		klog.Infof("[%s]Sync re-active", servername)
		syncer.connWarn.Store(servername, false)
	}
	// Update stream
	recordTime := time.Now()
	roomIdList := make([]int, 0, len(rooms))
	for _, room := range rooms {
		if !room.Recording {
			// Report recording rooms only
			continue
		}
		klog.Infof("room: %d", room.RoomID)
		room.RecordTime = recordTime
		roomIdList = append(roomIdList, room.RoomID)
		data, err := json.Marshal(&room)
		if err != nil {
			klog.Errorf("[%s]On marshal room(%d): %s", servername, room.RoomID, err.Error())
		}
		_, err = mq.Js.Publish(fmt.Sprintf("%s.history.%s.%d", syncer.StreamName, servername, room.RoomID), data)
		if err != nil {
			klog.Errorf("[%s]On update room(%d) stream: %s", servername, room.RoomID, err.Error())
		}
	}
	data, _ := json.Marshal(roomIdList)
	if _, err := mq.Js.Publish(fmt.Sprintf("%s.rooms.%s", syncer.StreamName, servername), data); err != nil {
		klog.Errorf("[%s]On update roomList stream: %s", servername, err.Error())
	}
	// klog.Infof("Updated %d rooms", len(roomIdList))
}
