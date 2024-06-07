package main

import (
	"time"
)

type BiliveRecorderRecord struct {
	RecordTime time.Time `json:"record_time"` // only available at pushing history(room is recording)
	BiliveRecorderRoom
}

type BiliveRecorderMetrics struct {
	NodeName        string                  `json:"node_name"`
	RequestDuration time.Duration           `json:"request_duration"`
	Rooms           []*BiliveRecorderRecord `json:"rooms"`
}

type BiliveRecorderRoom struct {
	ObjectID                 string                `json:"objectId"`
	RoomID                   int                   `json:"roomId"`
	AutoRecord               bool                  `json:"autoRecord"`
	ShortID                  int                   `json:"shortId"`
	Name                     string                `json:"name"`
	Title                    string                `json:"title"`
	AreaNameParent           string                `json:"areaNameParent"`
	AreaNameChild            string                `json:"areaNameChild"`
	Recording                bool                  `json:"recording"`
	Streaming                bool                  `json:"streaming"`
	DanmakuConnected         bool                  `json:"danmakuConnected"`
	AutoRecordForThisSession bool                  `json:"autoRecordForThisSession"`
	RecordingStats           BiliveRecorderStats   `json:"recordingStats"`
	IOStats                  BiliveRecorderIOStats `json:"ioStats"`
}

type BiliveRecorderStats struct {
	SessionDuration        float64 `json:"sessionDuration"`
	TotalInputBytes        int     `json:"totalInputBytes"`
	TotalOutputBytes       int     `json:"totalOutputBytes"`
	CurrentFileSize        int     `json:"currentFileSize"`
	SessionMaxTimestamp    int     `json:"sessionMaxTimestamp"`
	FileMaxTimestamp       int     `json:"fileMaxTimestamp"`
	AddedDuration          float64 `json:"addedDuration"`
	PassedTime             float64 `json:"passedTime"`
	DurationRatio          any     `json:"durationRatio"` // NaN or float
	InputVideoBytes        int     `json:"inputVideoBytes"`
	InputAudioBytes        int     `json:"inputAudioBytes"`
	OutputVideoFrames      int     `json:"outputVideoFrames"`
	OutputAudioFrames      int     `json:"outputAudioFrames"`
	OutputVideoBytes       int     `json:"outputVideoBytes"`
	OutputAudioBytes       int     `json:"outputAudioBytes"`
	TotalInputVideoBytes   int     `json:"totalInputVideoBytes"`
	TotalInputAudioBytes   int     `json:"totalInputAudioBytes"`
	TotalOutputVideoFrames int     `json:"totalOutputVideoFrames"`
	TotalOutputAudioFrames int     `json:"totalOutputAudioFrames"`
	TotalOutputVideoBytes  int     `json:"totalOutputVideoBytes"`
	TotalOutputAudioBytes  int     `json:"totalOutputAudioBytes"`
}

type BiliveRecorderIOStats struct {
	StreamHost             string    `json:"streamHost"`
	HostLookup             string    `json:"hostLookup"`
	StartTime              time.Time `json:"startTime"`
	EndTime                time.Time `json:"endTime"`
	Duration               float64   `json:"duration"`
	NetworkBytesDownloaded int       `json:"networkBytesDownloaded"`
	NetworkMbps            float64   `json:"networkMbps"`
	DiskWriteDuration      float64   `json:"diskWriteDuration"`
	DiskBytesWritten       int       `json:"diskBytesWritten"`
	DiskMBps               any       `json:"diskMBps"` // NaN or float
}

type BCdnDnsData struct {
	Version string                       `json:"time"`
	DNSList map[string][]*BCdnRegionNode `json:"dns"`
}

type BCdnRegionNode struct {
	ISP        string                    `json:"isp"`
	RegionCode string                    `json:"region_code"`
	RegionName string                    `json:"region_name"`
	Domains    map[string]*BCdnDomainIPs `json:"domains"`
}

type BCdnDomainIPs struct {
	IPv4 []string `json:"ipv4"`
	IPv6 []string `json:"ipv6"`
}

type BCdnDomainInfo struct {
	IPs        *BCdnDomainIPs
	RegionInfo *BCdnRegionNode
}

type RoomMetricsLabel struct {
	NodeID           string `json:"node_id"`
	RoomID           string `json:"room_id"`
	Name             string `json:"name"`
	Title            string `json:"title"`
	AreaNameParent   string `json:"area_name_parent"`
	AreaNameChild    string `json:"area_name_child"`
	StreamHost       string `json:"stream_host"`
	HostLookup       string `json:"host_lookup"`
	Recording        string `json:"recording"`
	Streaming        string `json:"streaming"`
	DanmakuConnected string `json:"danmaku_connected"`
}
