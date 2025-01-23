package main

import (
	"errors"
	"fmt"
	biliChat "github.com/FishZe/go-bili-chat/v2"
	biliChatClient "github.com/FishZe/go-bili-chat/v2/client"
	"github.com/FishZe/go-bili-chat/v2/events"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/nats-io/nats.go"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
	"sync"
	"sync/atomic"
	"time"
)

var (
	SupportedMsgTypes = []string{
		events.CmdDanmuMsg,
		events.CmdSendGift,
		events.CmdGuardBuy,
		events.CmdSuperChatMessage,
		events.CmdOnlineRankCount,
		events.CmdOnlineRankV2,
	}
	SupportedMsgTypesNames = map[string]string{
		events.CmdDanmuMsg:         "Damaku",
		events.CmdSendGift:         "Gift",
		events.CmdGuardBuy:         "Guard",
		events.CmdSuperChatMessage: "SuperChat",
		events.CmdOnlineRankCount:  "OnlineRank",
		events.CmdOnlineRankV2:     "OnlineRankV2",
	}
)

type DamakuCenterAgent struct {
	chatHandler   *biliChat.Handler
	controlChan   chan *nats.Msg
	eventChan     chan *BLiveEventHandlerMsg
	eventCounter  map[string]*atomic.Uint32
	watchingRooms sync.Map
	metaBuilder   agent.MetaBuilder

	// running channel
	userMetaChan  chan *agent.UserInfoMeta
	medalMetaChan chan *agent.FansMedalMeta
}

func (a *DamakuCenterAgent) Init() {
	a.chatHandler = biliChat.GetNewHandler()
	a.controlChan = make(chan *nats.Msg, 4)
	a.eventChan = make(chan *BLiveEventHandlerMsg, 100)
	a.metaBuilder = agent.NewMsgMetaBuilder(cfg.AgentId)
	a.userMetaChan = make(chan *agent.UserInfoMeta, 32)
	a.medalMetaChan = make(chan *agent.FansMedalMeta, 32)
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
	for _, msgType := range SupportedMsgTypes {
		a.eventCounter[msgType] = &atomic.Uint32{}
		a.chatHandler.AddOption(0, &BLiveEventHandlerWrapper{Command: msgType, EventChan: a.eventChan, Counter: a.eventCounter[msgType]})
	}
	go a.controller()
}

// collect status and receive control action
func (a *DamakuCenterAgent) controller() {
	collectTicker := time.NewTimer(time.Second)
	worker.Add(1)
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
			for k, counter := range a.eventCounter {
				status.BufferEventCount[SupportedMsgTypesNames[k]] = counter.Load()
			}
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
		case <-ctx.Done():
			klog.Infof("agent controller stopped")
			worker.Done()
			return
		}
	}
}

// can be parallelization
func (a *DamakuCenterAgent) eventHandler() {
	worker.Add(1)
	for {
		select {
		case msg := <-a.eventChan:
			msg.processTime = time.Now()
			roomId := uint64(msg.event.RoomId)
			switch msg.event.Cmd {
			case events.CmdDanmuMsg:
				// init
				userMeta := &agent.UserInfoMeta{}
				meta := a.metaBuilder()
				meta.RoomID = &roomId
				// parse
				data := gjson.ParseBytes(msg.event.RawMessage)
				meta.TimeStamp = data.Get("info.0.4").Uint()
				userMeta.UID = data.Get("info.2.0").Uint()
				userMeta.UserName = data.Get("info.2.1").String()
				userFace := data.Get("info.0.15.user.base.face").String()
				userMeta.Face = &userFace
				a.userMetaChan <- userMeta
				medal := &agent.FansMedalMeta{
					RoomUID:    data.Get("info.3.12").Uint(),
					Name:       data.Get("info.3.1").String(),
					Level:      uint32(data.Get("info.3.0").Uint()),
					Light:      data.Get("info.3.11").Bool(),
					GuardLevel: agent.GuardLevelType(data.Get("info.3.10").Uint()),
				}
				a.medalMetaChan <- medal
				danmaku := &agent.Damaku{
					Meta:    meta,
					UID:     userMeta.UID,
					Content: data.Get("info.1").String(),
					Medal:   medal.RoomUID,
				}
				// trace
				danmaku.Meta.Trace[int32(agent.TraceStep_Wait)] = uint64(msg.processTime.Sub(msg.startTime).Milliseconds())
				danmaku.Meta.Trace[int32(agent.TraceStep_Process)] = uint64(time.Now().Sub(msg.processTime).Milliseconds())
				sendData, err := proto.Marshal(danmaku)
				if err != nil {
					klog.Errorf("failed to marshal danmaku: %s", err.Error())
				}
				if err := mq.Publish(fmt.Sprintf("%s.agent.danmaku", cfg.SubjectPrefix), sendData); err != nil {
					klog.Errorf("publish danmaku message failed: %s", err.Error())
				}
			case events.CmdSendGift:
			case events.CmdGuardBuy:
			case events.CmdSuperChatMessage:
			case events.CmdOnlineRankCount:
			case events.CmdOnlineRankV2:
			default:
				klog.Warningf("unsupported command: %s", msg.event.Cmd)
				continue
			}
		case <-ctx.Done():
			klog.Infof("agent event handler stopped")
			worker.Done()
			return
		}
	}
}

// update & sync user/medal meta cache
func (a *DamakuCenterAgent) metaIndexer(meta *agent.UserInfoMeta) {

}
