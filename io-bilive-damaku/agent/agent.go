package main

import (
	"errors"
	"fmt"
	biliChat "github.com/FishZe/go-bili-chat/v2"
	biliChatClient "github.com/FishZe/go-bili-chat/v2/client"
	"github.com/FishZe/go-bili-chat/v2/events"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
	"sync"
	"time"
)

type DamakuCenterAgent struct {
	chatHandler   *biliChat.Handler
	controlChan   chan *nats.Msg
	eventChan     chan *events.BLiveEvent
	watchingRooms sync.Map
	metaBuilder   agent.MetaBuilder
}

func (a *DamakuCenterAgent) Init() {
	a.chatHandler = biliChat.GetNewHandler()
	a.controlChan = make(chan *nats.Msg, 4)
	a.eventChan = make(chan *events.BLiveEvent, 100)
	a.metaBuilder = agent.NewMsgMetaBuilder(cfg.AgentId)
	registerPacket := &agent.AgentInfo{ID: cfg.AgentId, Type: agent.AgentInfo_RealTimeAgent}
	registerData, err := proto.Marshal(registerPacket)
	if err != nil {
		klog.Fatalf("marshal register packet failed: %s", err.Error())
	}
	klog.Info("agent init started")
	registerChan := make(chan *nats.Msg, 1)
	registerSub, err := mq.Nc.ChanSubscribe(fmt.Sprintf("%s.agent.%s.init", cfg.SubjectPrefix, cfg.AgentId), registerChan)
	if err != nil {
		klog.Fatalf("subscribe init subject failed: %s", err.Error())
	}
	ticker := time.NewTicker(time.Second * 10)
initLoop:
	for {
		select {
		case <-ticker.C:
			if err := mq.Publish(fmt.Sprintf("%s.agent.info", cfg.SubjectPrefix), registerData); err != nil {
				klog.Errorf("publish register msg failed: %s", err.Error())
			}
		case msg := <-registerChan:
			regMsg := &agent.AgentInit{}
			if err := proto.Unmarshal(msg.Data, regMsg); err != nil {
				klog.Errorf("unmarshal register msg failed: %s", err.Error())
				continue
			}
			// setup bili client
			biliChat.SetHeaderCookie(regMsg.Cookie)
			biliChat.SetBuvid(regMsg.BUVID)
			biliChat.SetUID(int64(regMsg.UID))
			if regMsg.UA != nil {
				biliChat.SetHeaderUA(*regMsg.UA)
			}
			if regMsg.PriorityMode != nil {
				biliChat.SetClientPriorityMode(int(*regMsg.PriorityMode))
			}
			for k, v := range regMsg.Header {
				biliChatClient.Header.Set(k, v)
			}
			controlSub, err := mq.Nc.ChanSubscribe(fmt.Sprintf("%s.agent.%s.action", cfg.SubjectPrefix, cfg.AgentId), a.controlChan)
			if err != nil {
				klog.Errorf("subscribe control subject failed: %s", err.Error())
				_ = agent.ControlError(msg, err)
				continue
			}
			mq.AddSubscribe(controlSub)
			if err := agent.ControlSuccess(msg); err != nil {
				klog.Errorf("response control msg failed: %s", err.Error()) // is error will be ignored
			}
			ticker.Stop()
			_ = registerSub.Unsubscribe()
			break initLoop
		}
	}
	// start
	klog.Infof("agent initialized, UID: %d", biliChatClient.UID)
	// all supported handler here
	a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: events.CmdDanmuMsg, EventChan: a.eventChan})
	a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: events.CmdSendGift, EventChan: a.eventChan})
	a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: events.CmdGuardBuy, EventChan: a.eventChan})
	a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: events.CmdSuperChatMessage, EventChan: a.eventChan})
	a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: events.CmdOnlineRankCount, EventChan: a.eventChan})
	a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: events.CmdOnlineRankV2, EventChan: a.eventChan})
	go a.controller()
}

// collect status and receive control action
func (a *DamakuCenterAgent) controller() {
	collectTicker := time.NewTimer(time.Second)
	for {
		select {
		case <-collectTicker.C:
			status := &agent.AgentStatus{Meta: a.metaBuilder(), BufferUsed: uint32(len(a.eventChan))}
			a.watchingRooms.Range(func(key, _ any) bool {
				uKey, ok := key.(uint64)
				if !ok {
					klog.Errorf("key: %+v not a uint64", key)
					return true
				}
				status.Watching = append(status.Watching, uKey)
				return true
			})
			statusData, err := proto.Marshal(status)
			if err != nil {
				klog.Errorf("failed to marshal status data: %s", err.Error())
			}
			if err := mq.Publish("%s.agent.status", statusData); err != nil {
				klog.Errorf("publish status msg failed: %s", err.Error())
			}
		case controlMsg := <-a.controlChan:
			action := &agent.AgentAction{}
			if err := proto.Unmarshal(controlMsg.Data, action); err != nil {
				klog.Errorf("unmarshal agent action failed: %s", err.Error())
			}
			switch action.Type {
			case agent.AgentAction_AddRoom, agent.AgentAction_DelRoom:
				if action.RoomID == nil {
					klog.Errorf("no room id")
					if err := agent.ControlError(controlMsg, errors.New("no room id")); err != nil {
						klog.Errorf("response control msg failed: %s", err.Error())
					}
				}
				if *action.RoomID < 10000 {
					klog.Errorf("unsupported room id: %d", *action.RoomID)
					_ = agent.ControlError(controlMsg, fmt.Errorf("unsupported room id: %d", *action.RoomID))
				}
				if action.Type == agent.AgentAction_AddRoom {
					_ = a.chatHandler.AddRoom(int(*action.RoomID))
					a.watchingRooms.Store(*action.RoomID, struct{}{})
				} else if action.Type == agent.AgentAction_DelRoom {
					_ = a.chatHandler.DelRoom(int(*action.RoomID))
					a.watchingRooms.Delete(*action.RoomID)
				}
				if err := agent.ControlSuccess(controlMsg); err != nil {
					klog.Errorf("response control msg failed: %s", err.Error())
				}
			}
		}
	}
}

// can be parallelization
func (a *DamakuCenterAgent) eventHandler() {

}
