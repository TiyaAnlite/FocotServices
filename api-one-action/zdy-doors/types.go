package zdy_doors

import "encoding/json"

type CommonResponse struct {
	Code   int             `json:"errcode"`
	Msg    string          `json:"errmsg"`
	Result json.RawMessage `json:"result"`
}

type LoginResult struct {
	Token        string `json:"token"`
	YzxToken     string `json:"yzxToken"`
	CommName     string `json:"commName"`
	CommunityId  int    `json:"communityId"`
	UserId       int    `json:"userId"`
	SipUserId    string `json:"sipUserId"`
	SipPwd       string `json:"sipPwd"`
	SipServer    string `json:"sipServer"`
	SipPort      string `json:"sipPort"`
	EnableICE    int    `json:"enableICE"`
	EnableTURN   int    `json:"enableTURN"`
	TurnServer   string `json:"turnServer"`
	TurnUser     string `json:"turnUser"`
	TurnPassword string `json:"turnPassword"`
}

type DoorDevsItem struct {
	DevCode    string `json:"devCode"`
	DevName    string `json:"devName"`
	DevAddress string `json:"devAddress"`
	DevState   string `json:"devState"`
	DevType    string `json:"devType"`
}

type DoorDevsListResult struct {
	CommunityId int            `json:"communityId"`
	Doors       []DoorDevsItem `json:"doors"`
}
