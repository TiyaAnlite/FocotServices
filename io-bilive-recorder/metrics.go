package main

import (
	"fmt"
	"github.com/duke-git/lancet/v2/condition"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
	"net/http"
	"strconv"
	"time"
)

var (
	MetricsLabelNames = []string{"node_id", "room_id"}
)

type MetricsCollector interface {
	prometheus.Collector
	DeleteLabelValues(lvs ...string) bool
}

type MetricsExporter struct {
	UpdateChan   chan *BiliveRecorderMetrics
	registry     *prometheus.Registry
	collector    []MetricsCollector
	roomVersions map[string]map[int]time.Time // node_id:room_id:version
	// TODO: room info export support
	roomLabels map[string]map[int]prometheus.Labels // node_id:room_id:labels
	// metrics
	mAutoRecord             *prometheus.GaugeVec
	mRecording              *prometheus.GaugeVec
	mStreaming              *prometheus.GaugeVec
	mDanmakuConnected       *prometheus.GaugeVec
	mSessionDuration        *prometheus.GaugeVec
	mTotalInputBytes        *prometheus.GaugeVec
	mCurrentFileSize        *prometheus.GaugeVec
	mSessionMaxTimestamp    *prometheus.GaugeVec
	mFileMaxTimestamp       *prometheus.GaugeVec
	mNetworkBytesDownloaded *prometheus.GaugeVec
	mDiskBytesWritten       *prometheus.GaugeVec
	mNetworkMbps            *prometheus.GaugeVec
	mDiskMBps               *prometheus.GaugeVec
}

func (e *MetricsExporter) initMetrics() {
	e.registry = prometheus.NewRegistry()
	e.mAutoRecord = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_auto_record", Help: "units bool"}, MetricsLabelNames)
	e.mRecording = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_recording", Help: "units bool"}, MetricsLabelNames)
	e.mStreaming = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_streaming", Help: "units bool"}, MetricsLabelNames)
	e.mDanmakuConnected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_danmaku_connected", Help: "units bool"}, MetricsLabelNames)
	e.mSessionDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_session_duration", Help: "units ms"}, MetricsLabelNames)
	e.mTotalInputBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_total_input_bytes", Help: "units bytes"}, MetricsLabelNames)
	e.mCurrentFileSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_current_file_size", Help: "units bytes"}, MetricsLabelNames)
	e.mSessionMaxTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_session_max_timestamp", Help: "units ms"}, MetricsLabelNames)
	e.mFileMaxTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_file_max_timestamp", Help: "units ms"}, MetricsLabelNames)
	e.mNetworkBytesDownloaded = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_network_bytes_downloaded", Help: "units bytes"}, MetricsLabelNames)
	e.mDiskBytesWritten = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_disk_bytes_written", Help: "units bytes"}, MetricsLabelNames)
	e.mNetworkMbps = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_network_mbps", Help: "units bytes/s"}, MetricsLabelNames)
	e.mDiskMBps = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_disk_mbps", Help: "bytes/s"}, MetricsLabelNames)
	e.collector = []MetricsCollector{e.mAutoRecord, e.mRecording, e.mStreaming, e.mDanmakuConnected, e.mSessionDuration,
		e.mTotalInputBytes, e.mCurrentFileSize, e.mSessionMaxTimestamp, e.mFileMaxTimestamp, e.mNetworkBytesDownloaded,
		e.mDiskBytesWritten, e.mNetworkMbps, e.mDiskMBps}
	for _, collector := range e.collector {
		if err := e.registry.Register(collector); err != nil {
			klog.Fatalf("Register collector failed: %s", err.Error())
		}
	}
}

func (e *MetricsExporter) Start() {
	e.UpdateChan = make(chan *BiliveRecorderMetrics, 16)
	e.roomVersions = make(map[string]map[int]time.Time)
	e.roomLabels = make(map[string]map[int]prometheus.Labels)
	for node, _ := range cfg.Recorders {
		e.roomVersions[node] = make(map[int]time.Time)
		e.roomLabels[node] = make(map[int]prometheus.Labels)
	}
	e.initMetrics()
	http.Handle("/metrics", promhttp.HandlerFor(
		e.registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		}))
	go func() {
		klog.Infof("Prometheus exporter listening on port %d", cfg.ExporterPort)
		klog.Fatalln(http.ListenAndServe(fmt.Sprintf(":%d", cfg.ExporterPort), nil))
	}()
	go e.updateMetrics()
}

func (e *MetricsExporter) updateMetrics() {
	klog.Infof("[metrics]exporter started")
	for metrics := range e.UpdateChan {
		now := time.Now()
		// create & update
		for _, room := range metrics.Rooms {
			withValues := []string{metrics.NodeName, strconv.Itoa(room.RoomID)} // node_id:room_id
			e.mAutoRecord.WithLabelValues(withValues...).Set(float64(condition.TernaryOperator(room.AutoRecord, 1, 0)))
			e.mRecording.WithLabelValues(withValues...).Set(float64(condition.TernaryOperator(room.Recording, 1, 0)))
			e.mStreaming.WithLabelValues(withValues...).Set(float64(condition.TernaryOperator(room.Streaming, 1, 0)))
			e.mDanmakuConnected.WithLabelValues(withValues...).Set(float64(condition.TernaryOperator(room.DanmakuConnected, 1, 0)))
			e.mSessionDuration.WithLabelValues(withValues...).Set(room.RecordingStats.SessionDuration)
			e.mTotalInputBytes.WithLabelValues(withValues...).Set(float64(room.RecordingStats.TotalInputBytes))
			e.mCurrentFileSize.WithLabelValues(withValues...).Set(float64(room.RecordingStats.CurrentFileSize))
			e.mSessionMaxTimestamp.WithLabelValues(withValues...).Set(float64(room.RecordingStats.SessionMaxTimestamp))
			e.mFileMaxTimestamp.WithLabelValues(withValues...).Set(float64(room.RecordingStats.FileMaxTimestamp))
			e.mNetworkBytesDownloaded.WithLabelValues(withValues...).Set(float64(room.IOStats.NetworkBytesDownloaded))
			e.mDiskBytesWritten.WithLabelValues(withValues...).Set(float64(room.IOStats.DiskBytesWritten))
			e.mNetworkMbps.WithLabelValues(withValues...).Set(room.IOStats.NetworkMbps)
			e.mDiskMBps.WithLabelValues(withValues...).Set(func() float64 {
				if v, ok := room.IOStats.DiskMBps.(float64); ok {
					return v
				} else {
					return 0
				}
			}())
			e.roomVersions[metrics.NodeName][room.RoomID] = now
		}
		// scan & delete
		printRoom := true
		if len(metrics.Rooms) == 0 {
			printRoom = false // do not print when node whole offline
			klog.Infof("[metrics]clean expired node: %s", metrics.NodeName)
		}
		for roomID, v := range e.roomVersions[metrics.NodeName] {
			if v != now {
				if printRoom {
					klog.Infof("[metrics]clean expired record: node_id=%s, room_id=%d", metrics.NodeName, roomID)
				}
				for _, collector := range e.collector {
					collector.DeleteLabelValues(metrics.NodeName, strconv.Itoa(roomID))
				}
				delete(e.roomVersions[metrics.NodeName], roomID)
			}
		}
	}
}
