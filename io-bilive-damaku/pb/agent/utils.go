package agent

import (
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"time"
)

var (
	VERSION = uint32(1)
)

func ControlSuccess(controlMsg *nats.Msg) error {
	resp := &AgentControlResponse{Status: AgentControlResponse_OK}
	data, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	return controlMsg.Respond(data)
}

func ControlError(controlMsg *nats.Msg, err error) error {
	errStr := err.Error()
	resp := &AgentControlResponse{Status: AgentControlResponse_Err, Error: &errStr}
	data, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	return controlMsg.Respond(data)
}

type MetaBuilder func() *BasicMsgMeta

func NewMsgMetaBuilder(agentId string) MetaBuilder {
	return func() *BasicMsgMeta {
		return &BasicMsgMeta{
			Version:   VERSION,
			Agent:     agentId,
			TimeStamp: uint64(time.Now().UnixMilli()),
		}
	}
}
