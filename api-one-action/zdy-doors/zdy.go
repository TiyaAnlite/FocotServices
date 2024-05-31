package zdy_doors

import (
	"encoding/json"
	"fmt"
	"github.com/levigross/grequests"
	"k8s.io/klog/v2"
)

const (
	SrvUrl         = "http://39.97.176.54:8089"
	UrlLogin       = "/AppAPI/login"
	UrlGetDoorDevs = "/AppAPI/GetDoorDevs"
	UrlOpenDoor    = "/AppAPI/OpenDoor"
)

func parseResponse[T any](response *CommonResponse) (*T, error) {
	var result *T
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("result is null")
	}
	return result, nil
}

type requestOption struct {
	CheckErrCode bool `json:"check_err_code"` // return error when errCode != 0
	Auth         bool `json:"auth"`           // auto-inject Accesstoken params
	AutoRefresh  bool `json:"auto_refresh"`   // auto refresh when errCode == 2
}

type RequestOptionFunc func(*requestOption)

func WithCheckErrCode(check bool) RequestOptionFunc {
	return func(option *requestOption) {
		option.CheckErrCode = check
	}
}

func WithAuth(auth bool) RequestOptionFunc {
	return func(option *requestOption) {
		option.Auth = auth
	}
}

func WithAutoRefresh(refresh bool) RequestOptionFunc {
	return func(option *requestOption) {
		option.AutoRefresh = refresh
	}
}

func (srv *ZdyService) doRequest(method, url string, opt *grequests.RequestOptions, reqOpts ...RequestOptionFunc) (*CommonResponse, error) {
	reqOpt := requestOption{
		CheckErrCode: true,
		Auth:         true,
		AutoRefresh:  true,
	}
	for _, optFunc := range reqOpts {
		optFunc(&reqOpt)
	}
	if reqOpt.Auth && srv.token == "" {
		reqOpt.AutoRefresh = false
		if _, err := srv.refreshToken(); err != nil {
			return nil, err
		}
	}
	// inject token
	if opt.Data != nil {
		opt.Data["Accesstoken"] = srv.token
	} else {
		klog.Warning("cannot inject token because data is null")
	}
	resp, err := grequests.Req(method, url, opt)
	if err != nil {
		return nil, err
	}
	var r *CommonResponse
	if err := resp.JSON(&r); err != nil {
		return nil, err
	}
	if r.Code == 2 && reqOpt.AutoRefresh {
		_, err := srv.refreshToken()
		if err != nil {
			return nil, err
		}
		return srv.doRequest(method, url, opt, WithCheckErrCode(reqOpt.CheckErrCode), WithAuth(reqOpt.AutoRefresh)) // retry
	}
	if reqOpt.CheckErrCode {
		if r.Code != 0 {
			return nil, fmt.Errorf("code %d: %s", r.Code, r.Msg)
		}
	}
	return r, nil
}

func (srv *ZdyService) refreshToken() (string, error) {
	ro := &grequests.RequestOptions{
		Data: map[string]string{
			"phone":    cfg.Phone,
			"password": cfg.Password,
		},
	}
	resp, err := srv.doRequest("POST", SrvUrl+UrlLogin, ro,
		WithCheckErrCode(true), WithAuth(false), WithAutoRefresh(false))
	if err != nil {
		return "", fmt.Errorf("login failed: %s", err.Error())
	}
	res, err := parseResponse[LoginResult](resp)
	if err != nil {
		return "", fmt.Errorf("login failed: parse response failed: %s", err.Error())
	}
	klog.Infof("zdy user login: %d", res.UserId)
	srv.token = res.Token
	srv.communityId = res.CommunityId
	return res.Token, nil
}
