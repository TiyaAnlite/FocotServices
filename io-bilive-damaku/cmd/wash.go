package main

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/agent/parse"
	"github.com/TiyaAnlite/FocotServices/io-bilive-damaku/pb/agent"
	"github.com/bytedance/sonic"
	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/urfave/cli/v2"
	"k8s.io/klog/v2"
)

var WashApp = &WashCommand{}

type WashCommand struct {
}

func (w *WashCommand) Command() *cli.Command {
	return &cli.Command{
		Name:            "wash",
		Usage:           "washing BililiveRecorder raw xml damaku data to structured data",
		HideHelpCommand: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "dir",
				Aliases: []string{"d"},
				Value:   "./",
				Usage:   "Directory to be processed",
			},
			&cli.BoolFlag{
				Name:    "recursive",
				Aliases: []string{"r"},
				Usage:   "whether to include subdirectories",
				Value:   false,
			},
			&cli.StringFlag{
				Name:    "pattern",
				Aliases: []string{"p"},
				Usage:   "file name pattern to be processed",
				Value:   "^Blive-\\d+-\\d+-\\d+-\\d+-\\S+.xml",
			},
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "only file to be processed and force overwrite",
			},
			&cli.BoolFlag{
				Name:  "no-compress",
				Usage: "whether to compress the output file",
				Value: false,
			},
		},
		Action: w.action,
	}
}

func (w *WashCommand) action(c *cli.Context) error {
	var fileList []string
	if onlyFile := c.String("file"); onlyFile != "" {
		fileList = append(fileList, onlyFile)
	} else {
		// adding files
		klog.Infof("scanning dir: %s", c.String("dir"))
		pattern := regexp.MustCompile(c.String("pattern"))
		if c.Bool("recursive") {
			err := filepath.Walk(c.String("dir"), func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && pattern.MatchString(info.Name()) {
					if fileutil.IsExist(strings.TrimSuffix(path, filepath.Ext(path))+".json") ||
						fileutil.IsExist(strings.TrimSuffix(path, filepath.Ext(path))+".json.gz") {
						return nil
					}
					fileList = append(fileList, path)
				}
				return nil
			})
			if err != nil {
				klog.Errorf("walk dir error: %s", err.Error())
				return err
			}
		} else {
			dir := c.String("dir")
			entry, err := os.ReadDir(dir)
			if err != nil {
				klog.Errorf("read dir error: %s", err.Error())
				return err
			}
			for _, e := range entry {
				if e.IsDir() {
					continue
				}
				if pattern.MatchString(e.Name()) {
					if fileutil.IsExist(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))+".json") ||
						fileutil.IsExist(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))+".json.gz") {
						continue
					}
					fileList = append(fileList, filepath.Join(dir, e.Name()))
				}
			}
		}
		klog.Infof("scan finished, %d files found", len(fileList))
	}

	compress := !c.Bool("no-compress")

	for _, f := range fileList {
		w.washer(f, compress)
	}
	return nil
}

func (w *WashCommand) washer(filename string, compress bool) {
	klog.Infof("processing file: %s", filename)
	srcFp, err := os.Open(filename)
	if err != nil {
		klog.Errorf("open file error: %s", err.Error())
		return
	}
	defer srcFp.Close()
	srcBuf := bufio.NewReader(srcFp)

	decoder := xml.NewDecoder(srcBuf)
	var data BliveData
	data.mapUser = make(map[uint64]*agent.UserInfoMeta)
	data.mapMedal = make(map[uint64]map[uint64]*agent.FansMedalMeta)
	for {
		token, err := decoder.Token()
		if token == nil && err == io.EOF {
			break
		}
		if err != nil {
			klog.Errorf("decode xml error: %s", err.Error())
			return
		}
		w.attrParse(&data, token)
	}
	// final map listing
	for _, u := range data.mapUser {
		data.User = append(data.User, u)
	}
	for _, m := range data.mapMedal {
		for _, rm := range m {
			data.FansMedal = append(data.FansMedal, rm)
		}
	}

	var rw io.WriteCloser
	if compress {
		dstFp, err := os.Create(strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json.gz")
		if err != nil {
			klog.Errorf("create file error: %s", err.Error())
		}
		defer dstFp.Close()
		rw = gzip.NewWriter(dstFp)
	} else {
		rw, err = os.Create(strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json")
		if err != nil {
			klog.Errorf("create file error: %s", err.Error())
		}
	}
	defer rw.Close()
	err = sonic.ConfigDefault.NewEncoder(rw).Encode(data)
	if err != nil {
		klog.Errorf("encode json error: %s", err.Error())
	}
}

func (w *WashCommand) attrParse(data *BliveData, token xml.Token) {
	switch t := token.(type) {
	case xml.StartElement:
		switch t.Name.Local {
		case "BililiveRecorder":
			for _, attr := range t.Attr {
				if attr.Name.Local == "version" {
					data.Meta.RecorderVersion = attr.Value
				}
			}
		case "BililiveRecorderRecordInfo":
			for _, attr := range t.Attr {
				switch attr.Name.Local {
				case "roomid":
					data.Meta.RoomID, _ = strconv.ParseInt(attr.Value, 10, 64)
				case "shortid":
					data.Meta.ShortRoomID, _ = strconv.ParseInt(attr.Value, 10, 64)
				case "name":
					data.Meta.Name = attr.Value
				case "title":
					data.Meta.Title = attr.Value
				case "areanameparent":
					data.Meta.AreaNameParent = attr.Value
				case "areanamechild":
					data.Meta.AreaNameChild = attr.Value
				case "start_time":
					data.Meta.StartTime, _ = time.Parse(time.RFC3339, attr.Value)
				}
			}
		case "d":
			// damaku
			for _, attr := range t.Attr {
				if attr.Name.Local == "raw" {
					var user agent.UserInfoMeta
					var medal agent.FansMedalMeta
					var damaku agent.Damaku
					damaku.Meta = &agent.BasicMsgMeta{}
					parse.Danmu(attr.Value, &user, &medal, &damaku)
					data.Damaku = append(data.Damaku, &damaku)
					w.updateUser(data, &user)
					w.updateMedal(data, &medal)
				}
			}
		case "gift":
			// gift
			for _, attr := range t.Attr {
				if attr.Name.Local == "raw" {
					var user agent.UserInfoMeta
					var medal agent.FansMedalMeta
					var gift agent.Gift
					gift.Meta = &agent.BasicMsgMeta{}
					parse.Gift(attr.Value, &user, &medal, &gift)
					data.Gift = append(data.Gift, &gift)
					w.updateUser(data, &user)
					w.updateMedal(data, &medal)
				}
			}
		case "sc":
			// superchat
			for _, attr := range t.Attr {
				if attr.Name.Local == "raw" {
					var user agent.UserInfoMeta
					var medal agent.FansMedalMeta
					var sc agent.SuperChat
					sc.Meta = &agent.BasicMsgMeta{}
					parse.SuperChat(attr.Value, &user, &medal, &sc)
					data.SuperChat = append(data.SuperChat, &sc)
					w.updateUser(data, &user)
					w.updateMedal(data, &medal)
				}
			}
		case "guard":
			// guard
			for _, attr := range t.Attr {
				if attr.Name.Local == "raw" {
					var user agent.UserInfoMeta
					var guard agent.Guard
					guard.Meta = &agent.BasicMsgMeta{}
					parse.Guard(attr.Value, &user, &guard)
					data.Guard = append(data.Guard, &guard)
					w.updateUser(data, &user)
				}
			}
		}
	}
}

func (w *WashCommand) updateUser(data *BliveData, userInfo *agent.UserInfoMeta) {
	info, ok := data.mapUser[userInfo.UID]
	if !ok {
		info = &agent.UserInfoMeta{}
		data.mapUser[userInfo.UID] = info
	}
	info.UID = userInfo.UID
	info.UserName = userInfo.UserName
	switch {
	case userInfo.Face != nil:
		info.Face = userInfo.Face
	case userInfo.Level != nil:
		info.Level = userInfo.Level
	case userInfo.WealthLevel != nil:
		info.WealthLevel = userInfo.WealthLevel
	}
}

func (w *WashCommand) updateMedal(data *BliveData, medalInfo *agent.FansMedalMeta) {
	if medalInfo.RoomUID == 0 {
		// no medal
		return
	}
	medal, ok := data.mapMedal[medalInfo.UID]
	if !ok {
		medal = make(map[uint64]*agent.FansMedalMeta)
		data.mapMedal[medalInfo.UID] = medal
	}
	roomMedal, ok := medal[medalInfo.RoomUID]
	if !ok {
		roomMedal = &agent.FansMedalMeta{
			UID:     medalInfo.UID,
			RoomUID: medalInfo.RoomUID,
		}
		medal[medalInfo.RoomUID] = roomMedal
	}
	roomMedal.Name = medalInfo.Name
	roomMedal.Level = medalInfo.Level
	roomMedal.Light = medalInfo.Light
	roomMedal.GuardLevel = medalInfo.GuardLevel
}
