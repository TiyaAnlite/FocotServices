package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/levigross/grequests"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

type taskStatus struct {
	Padding        bool
	client         *grequests.Session
	streamingState map[int]bool
	recordingState map[int]bool
	connWarn       bool
	counter        int
	tracer         trace.Span
	taskCtx        context.Context
}

type DataSyncer struct {
	Recorders       map[string]string
	RefreshInterval int
	MaxRequestTime  int
	StreamName      string
	taskStatus      map[string]*taskStatus
	bcdn            *BCdnDnsData
	bcdnRegions     map[string]*BCdnDomainInfo
	syncStarted     bool
	wg              *worker
	mu              sync.RWMutex
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
	syncer.taskStatus = make(map[string]*taskStatus, len(syncer.Recorders))
	for serverName := range syncer.Recorders {
		syncer.taskStatus[serverName] = &taskStatus{
			Padding:        false,
			streamingState: make(map[int]bool),
			recordingState: make(map[int]bool),
			connWarn:       false,
			counter:        0,
		}
	}
	ticker := time.NewTicker(time.Duration(int64(time.Second) * int64(syncer.RefreshInterval)))
	for {
		select {
		case <-ticker.C:
			for serverName, status := range syncer.taskStatus {
				if status.Padding {
					// Task padding
					continue
				} else {
					url, ok := syncer.Recorders[serverName]
					if !ok {
						klog.Errorf("Unknown recorders: %s", serverName)
						continue
					}
					// klog.Infof("Sync %s", serverName)
					go syncer.doRequest(serverName, url)
				}
			}
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
	status := syncer.taskStatus[servername]
	status.Padding = true
	defer func() {
		status.counter++
		if status.counter >= 100 {
			status.tracer.End()
			status.taskCtx = nil
			status.tracer = nil
			status.counter = 0
		}
		status.Padding = false
	}()
	if status.tracer == nil {
		// First trace
		status.taskCtx, status.tracer = syncer.wg.Start(syncer.wg.Ctx, "doSyncRequestP100")
		status.tracer.SetAttributes(attribute.String("bilive.servername", servername))
	}
	if status.client == nil {
		status.tracer.AddEvent("New client")
		klog.Infof("[%s]New http client", servername)
		ro := &grequests.RequestOptions{
			RequestTimeout: time.Duration(int64(time.Second) * int64(syncer.MaxRequestTime)),
			DialKeepAlive:  time.Second * 30,
			Context:        syncer.wg.Ctx,
		}
		status.client = grequests.NewSession(ro)
	}
	_, apiTrace := syncer.wg.Start(status.taskCtx, "apiRequest")
	startTime := time.Now()
	apiTrace.SetAttributes(attribute.String("bilive.recorderUrl", recorderUrl))
	defer apiTrace.End()
	resp, err := status.client.Get(fmt.Sprintf("http://%s/api/room", recorderUrl), nil)
	duration := time.Since(startTime)
	metrics := BiliveRecorderMetrics{
		NodeName:        servername,
		RequestDuration: duration,
	}
	if err != nil {
		if !status.connWarn {
			apiTrace.RecordError(err)
			klog.Errorf("[%s]On sync: %s", servername, err.Error())
			status.connWarn = true
		}
		exporter.UpdateChan <- &metrics // failed update
		return
	}
	apiTrace.End()
	_, parseTrace := syncer.wg.Start(status.taskCtx, "biliveRoomsParse")
	defer parseTrace.End()
	var rooms []*BiliveRecorderRecord
	if err := resp.JSON(&rooms); err != nil {
		parseTrace.RecordError(err)
		klog.Errorf("[%s]On parse rooms: %s", servername, err.Error())
		exporter.UpdateChan <- &metrics // failed update
		return
	}
	// Lookup stream domain
	lookupList := make([]string, len(rooms))
	for i, room := range rooms {
		lookupList[i] = room.IOStats.StreamHost
	}
	lookupResult := syncer.BCdnLookupBatch(lookupList)
	for i, room := range rooms {
		if lookupResult[i] != nil {
			var isp string
			var ok bool
			if isp, ok = ISPName[lookupResult[i].RegionInfo.ISP]; !ok {
				isp = lookupResult[i].RegionInfo.ISP
			}
			room.IOStats.HostLookup = fmt.Sprintf("%s-%s", lookupResult[i].RegionInfo.RegionName, isp)
		}
	}
	metrics.Rooms = rooms
	exporter.UpdateChan <- &metrics // successful update
	parseTrace.End()
	if status.connWarn {
		status.tracer.AddEvent("Sync re-active")
		klog.Infof("[%s]Sync re-active", servername)
		status.connWarn = false
	}
	// Update stream
	_, natsTrace := syncer.wg.Start(status.taskCtx, "updateJetStream")
	natsTrace.SetAttributes(attribute.Int("bilive.fetchRoom", len(rooms)))
	defer natsTrace.End()
	recordTime := time.Now()
	roomIdList := make([]int, 0, len(rooms))
	updated := 0
	updateError := 0
	for _, room := range rooms {
		streamingState, ok := status.streamingState[room.RoomID]
		if !ok || streamingState != room.Streaming {
			klog.Infof("[%s](%d){%s}Stream state changing to: %t", servername, room.RoomID, room.Name, room.Streaming)
			status.streamingState[room.RoomID] = room.Streaming
		}
		recordingState, ok := status.recordingState[room.RoomID]
		if !ok || recordingState != room.Recording {
			klog.Infof("[%s](%d){%s}Record state changing to: %t", servername, room.RoomID, room.Name, room.Recording)
			status.recordingState[room.RoomID] = room.Recording
		}
		if !room.Recording {
			// Report recording rooms only
			continue
		}
		// klog.Infof("room: %d", room.RoomID)
		room.RecordTime = recordTime
		roomIdList = append(roomIdList, room.RoomID)
		data, err := json.Marshal(&room)
		if err != nil {
			natsTrace.RecordError(err)
			klog.Errorf("[%s]On marshal room(%d): %s", servername, room.RoomID, err.Error())
			updateError++
			continue
		}
		_, err = mq.Js.Publish(fmt.Sprintf("%s.history.%s.%d", syncer.StreamName, servername, room.RoomID), data)
		if err != nil {
			natsTrace.RecordError(err)
			klog.Errorf("[%s]On update room(%d) stream: %s", servername, room.RoomID, err.Error())
			updateError++
		} else {
			updated++
		}
	}
	natsTrace.SetAttributes(attribute.Int("bilive.updated", updated), attribute.Int("bilive.updateError", updateError))
	data, _ := json.Marshal(roomIdList)
	if _, err := mq.Js.Publish(fmt.Sprintf("%s.rooms.%s", syncer.StreamName, servername), data); err != nil {
		natsTrace.RecordError(err)
		klog.Errorf("[%s]On update roomList stream: %s", servername, err.Error())
	}
	natsTrace.End()
	// klog.Infof("Updated %d rooms", len(roomIdList))
}
