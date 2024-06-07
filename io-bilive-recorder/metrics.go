package main

import (
	"fmt"
	"github.com/duke-git/lancet/v2/condition"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
	"net/http"
	"reflect"
	"strconv"
	"time"
)

var (
	MetricsLabelNames     = []string{"node_id", "room_id"}
	MetricsRoomLabelNames = (&RoomMetricsLabel{}).Keys()
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
	roomLabels   map[string]map[int][]string  // node_id:room_id:labels
	// metrics
	mRoomInfo               *prometheus.GaugeVec
	mRequestDuration        *prometheus.HistogramVec
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
	e.mRoomInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "blive_recorder_room_info"}, MetricsRoomLabelNames)
	e.mRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "blive_request_duration", Help: "units ms"}, []string{"node_id"})
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
	e.registry.MustRegister(e.mRoomInfo, e.mRequestDuration) // unmanaged from room collector
	for _, collector := range e.collector {
		if err := e.registry.Register(collector); err != nil {
			klog.Fatalf("Register collector failed: %s", err.Error())
		}
	}
}

func (e *MetricsExporter) Start() {
	e.UpdateChan = make(chan *BiliveRecorderMetrics, 16)
	e.roomVersions = make(map[string]map[int]time.Time)
	e.roomLabels = make(map[string]map[int][]string)
	for node, _ := range cfg.Recorders {
		e.roomVersions[node] = make(map[int]time.Time)
		e.roomLabels[node] = make(map[int][]string)
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
		e.mRequestDuration.WithLabelValues(metrics.NodeName).Observe(float64(metrics.RequestDuration.Milliseconds()))
		// create & update
		for _, room := range metrics.Rooms {
			roomID := strconv.Itoa(room.RoomID)

			// room info
			roomInfo := RoomMetricsLabel{
				NodeID:           metrics.NodeName,
				RoomID:           roomID,
				Name:             room.Name,
				Title:            room.Title,
				AreaNameParent:   room.AreaNameParent,
				AreaNameChild:    room.AreaNameChild,
				StreamHost:       room.IOStats.StreamHost,
				HostLookup:       room.IOStats.HostLookup,
				Recording:        condition.TernaryOperator(room.Recording, "1", "0"),
				Streaming:        condition.TernaryOperator(room.Streaming, "1", "0"),
				DanmakuConnected: condition.TernaryOperator(room.DanmakuConnected, "1", "0"),
			}
			roomInfoValues := roomInfo.Values()
			if oldInfo, ok := e.roomLabels[metrics.NodeName][room.RoomID]; !ok || !slice.Equal(roomInfoValues, oldInfo) {
				klog.Infof("[metrics]update roomInfo[%d] from node[%s]", room.RoomID, metrics.NodeName)
				// need create(!ok) or update(!equal)
				if ok {
					// !equal
					e.mRoomInfo.DeleteLabelValues(oldInfo...)
				}
				e.mRoomInfo.WithLabelValues(roomInfoValues...).Set(1)
				e.roomLabels[metrics.NodeName][room.RoomID] = roomInfoValues
			}

			// room metrics
			withValues := []string{metrics.NodeName, roomID} // node_id:room_id
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
			// expired node
			printRoom = false // do not print when node whole offline
			if len(e.roomVersions[metrics.NodeName]) != 0 {
				klog.Infof("[metrics]clean expired node: %s", metrics.NodeName)
			}
			// clean room info
			for _, oldInfo := range e.roomLabels[metrics.NodeName] {
				e.mRoomInfo.DeleteLabelValues(oldInfo...)
			}
		}
		// clean room metrics
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

func (l *RoomMetricsLabel) fields() (fields []reflect.StructField) {
	v := reflect.ValueOf(l).Elem()
	for i := 0; i < v.NumField(); i++ {
		fieldInfo := v.Type().Field(i)
		if !fieldInfo.IsExported() {
			continue
		}
		fields = append(fields, fieldInfo)
	}
	return
}

func (l *RoomMetricsLabel) Keys() (keys []string) {
	for _, field := range l.fields() {
		keys = append(keys, field.Tag.Get("json"))
	}
	return
}

func (l *RoomMetricsLabel) Values() (values []string) {
	value := reflect.ValueOf(l).Elem()
	for _, field := range l.fields() {
		v := value.FieldByName(field.Name)
		values = append(values, v.String())
	}
	return
}
