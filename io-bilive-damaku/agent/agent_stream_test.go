package main

import (
	"fmt"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"github.com/bytedance/sonic"
	"github.com/duke-git/lancet/v2/xerror"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

type TestConfig struct {
	BUVID    string `json:"buvid" env:"BUVID,required"`
	UID      uint64 `json:"uid" env:"UID,required"`
	Cookie   string `json:"cookie" env:"COOKIE,required"`
	TestRoom uint64 `json:"test_room" env:"TEST_ROOM,required"`
}

var (
	autoResponseMsg = xerror.TryUnwrap(proto.Marshal(&agent.AgentControlResponse{Status: agent.AgentControlResponse_OK}))
)

func TestAgentFunc(t *testing.T) {
	testCfg := &TestConfig{}
	envx.MustLoadEnv(testCfg)
	t.Log("Waiting agent...")
	if err := mq.Open(cfg.NatsConfig); err != nil {
		t.Fatalf("Cannot connect to NATS: %s", err.Error())
	}
	defer mq.Close()
	BindEvent(mq.Nc, cfg.SubjectPrefix, cfg.AgentId)
	t.Logf("using subject prefix: %s", cfg.SubjectPrefix)
	sub, err := mq.Nc.SubscribeSync(fmt.Sprintf("%s.agent.info", cfg.SubjectPrefix))
	if err != nil {
		t.Fatalf("Cannot subscribe to subject: %s", err.Error())
	}
	msg, err := sub.NextMsg(time.Second * 30)
	if err != nil {
		t.Fatalf("Agent init failed: %s", err.Error())
	}
	var initInfo agent.AgentInfo
	if err := proto.Unmarshal(msg.Data, &initInfo); err != nil {
		t.Fatalf("Cannot unmarshal init info: %s", err.Error())
	}
	if initInfo.ID != cfg.AgentId {
		t.Fatalf("Agent init info mismatch, need: %s, got: %s", cfg.AgentId, initInfo.ID)
	}
	initMsg := &agent.AgentInit{
		BUVID:  testCfg.BUVID,
		UID:    testCfg.UID,
		Cookie: testCfg.Cookie,
	}
	msg = xerror.TryUnwrap(
		mq.Request(
			fmt.Sprintf("%s.agent.%s.init", cfg.SubjectPrefix, cfg.AgentId),
			xerror.TryUnwrap(proto.Marshal(initMsg)),
			time.Second))
	var controlResp agent.AgentControlResponse
	if err := proto.Unmarshal(msg.Data, &controlResp); err != nil {
		t.Fatalf("Cannot unmarshal control response: %s", err.Error())
	}
	if controlResp.Status != agent.AgentControlResponse_OK {
		t.Fatalf("Agent init failed: %v", controlResp.Error)
	}
	t.Logf("agent init, send room: %d", testCfg.TestRoom)
	control := &agent.AgentAction{
		Type:   agent.AgentAction_AddRoom,
		RoomID: &testCfg.TestRoom,
	}
	msg = xerror.TryUnwrap(
		mq.Request(
			fmt.Sprintf("%s.agent.%s.action", cfg.SubjectPrefix, cfg.AgentId),
			xerror.TryUnwrap(proto.Marshal(control)),
			time.Second))
	if controlResp.Status != agent.AgentControlResponse_OK {
		t.Fatalf("Agent room add failed: %v", controlResp.Error)
	}
	utils.Wait4CtrlC()
}

func EventShow(event proto.Message) func(msg *nats.Msg) {
	return func(msg *nats.Msg) {
		if err := proto.Unmarshal(msg.Data, event); err != nil {
			fmt.Println("Cannot unmarshal event: " + err.Error())
		}
		fmt.Println(fmt.Sprintf("[EventShow]%T: %s", event, xerror.TryUnwrap(sonic.Marshal(&event))))
	}
}

func AutoResponseEventShow(event proto.Message) func(msg *nats.Msg) {
	return func(msg *nats.Msg) {
		EventShow(event)(msg)
		_ = msg.Respond(autoResponseMsg)
	}
}

func BindEvent(mq *nats.Conn, prefix, agentId string) {
	_, _ = mq.Subscribe(prefix+".agent.info", EventShow(&agent.AgentInfo{}))
	_, _ = mq.Subscribe(prefix+".agent."+agentId+".init", EventShow(&agent.AgentInit{}))
	_, _ = mq.Subscribe(prefix+".agent."+agentId+".action", EventShow(&agent.AgentAction{}))
	_, _ = mq.Subscribe(prefix+".agent.status", EventShow(&agent.AgentStatus{}))
	_, _ = mq.Subscribe(prefix+".stream.fansMedal", AutoResponseEventShow(&agent.FansMedalMeta{}))
	_, _ = mq.Subscribe(prefix+".stream.userInfoMeta", AutoResponseEventShow(&agent.UserInfoMeta{}))
	_, _ = mq.Subscribe(prefix+".stream.damaku", EventShow(&agent.Damaku{}))
	_, _ = mq.Subscribe(prefix+".stream.gift", EventShow(&agent.Gift{}))
	_, _ = mq.Subscribe(prefix+".stream.guard", EventShow(&agent.Guard{}))
	_, _ = mq.Subscribe(prefix+".stream.superChat", EventShow(&agent.SuperChat{}))
	_, _ = mq.Subscribe(prefix+".stream.online", EventShow(&agent.OnlineRankCount{}))
	_, _ = mq.Subscribe(prefix+".stream.onlineV2", EventShow(&agent.OnlineRankV2{}))
}
