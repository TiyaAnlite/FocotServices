package main

import "github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"

type AgentManager struct {
}

func (m *AgentManager) OnAgentInfo(info *agent.AgentInfo) {

}

func (m *AgentManager) OnAgentStatus(status *agent.AgentStatus) {

}

// AgentMask return unique agent binary mask using for cache or identifier
// if agent not exist, will return nil
func (m *AgentManager) AgentMask(agentId string) []byte {
	return nil
}

// AddAgentHitMask will update agent message counter
// the masks must a serial of AgentMask
func (m *AgentManager) AddAgentHitMask(masks []byte) {

}
