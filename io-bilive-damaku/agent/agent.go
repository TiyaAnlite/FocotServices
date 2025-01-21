package main

import (
	"fmt"
	biliChat "github.com/FishZe/go-bili-chat/v2"
	biliChatClient "github.com/FishZe/go-bili-chat/v2/client"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"k8s.io/klog/v2"
	"time"
)

type DamakuCenterAgent struct {
	handler *biliChat.Handler
}

func (a *DamakuCenterAgent) Init() {
	registerPacket := &agent.AgentInfo{ID: cfg.AgentId, Type: agent.AgentInfo_RealTimeAgent}
	registerData, err := proto.Marshal(registerPacket)
	if err != nil {
		klog.Fatalf("marshal register packet failed: %s", err.Error())
	}
	klog.Info("agent init started")
	registerChan := make(chan *nats.Msg, 1)
	registerSub, err := mq.Nc.ChanSubscribe(fmt.Sprintf("%s.%s.init", cfg.SubjectPrefix, cfg.AgentId), registerChan)
	if err != nil {
		klog.Fatalf("subscribe init subject failed: %s", err.Error())
	}
	timer := time.NewTimer(time.Second * 10)
initLoop:
	for {
		select {
		case <-timer.C:
			if err := mq.Publish(fmt.Sprintf("%s.info", cfg.SubjectPrefix), registerData); err != nil {
				klog.Errorf("publish register msg failed: %s", err.Error())
			}
		case msg := <-registerChan:
			regMsg := &agent.AgentInit{}
			if err := proto.Unmarshal(msg.Data, regMsg); err != nil {
				klog.Errorf("unmarshal register msg failed: %s", err.Error())
			}
			timer.Stop()
			_ = registerSub.Unsubscribe()
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
			if err := agent.ControlSuccess(msg); err != nil {
				klog.Errorf("response control msg failed: %s", err.Error()) // is error will be ignored
			}
			break initLoop
		}
	}
	// start
	klog.Infof("agent initialized, UID: %d", biliChatClient.UID)
	a.handler = biliChat.GetNewHandler()
	// TODO: Add handler option
	go a.controller()
}

// collect status and receive control action
func (a *DamakuCenterAgent) controller() {
	collectTimer := time.NewTimer(time.Second)
	_ = collectTimer
}
