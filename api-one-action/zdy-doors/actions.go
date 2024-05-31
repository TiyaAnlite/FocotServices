package zdy_doors

import (
	"github.com/TiyaAnlite/FocotServicesCommon/echox"
	"github.com/labstack/echo/v4"
	"github.com/levigross/grequests"
	"k8s.io/klog/v2"
	"net/http"
	"strconv"
)

func listDoorDevs(c echo.Context) error {
	if instance.communityId == 0 {
		if _, err := instance.refreshToken(); err != nil {
			klog.Errorf("refreshToken failed: %s", err.Error())
			return echox.NormalErrorResponse(c, http.StatusInternalServerError, http.StatusInternalServerError, "listDoorDevs failed")
		}
	}
	opt := &grequests.RequestOptions{
		Data: map[string]string{
			"Community": strconv.Itoa(instance.communityId),
		},
	}
	resp, err := instance.doRequest("POST", SrvUrl+UrlGetDoorDevs, opt)
	if err != nil {
		klog.Errorf("listDoorDevs failed: %s", err.Error())
		return echox.NormalErrorResponse(c, http.StatusInternalServerError, http.StatusInternalServerError, "listDoorDevs failed")
	}
	doors, err := parseResponse[[]DoorDevsItem](resp)
	if err != nil {
		klog.Errorf("parse doors failed: %s", err.Error())
		return echox.NormalErrorResponse(c, http.StatusInternalServerError, http.StatusInternalServerError, "parse doors failed")
	}
	return echox.NormalResponse(c, &DoorDevsListResult{
		Doors:       *doors,
		CommunityId: instance.communityId,
	})
}

func openDoor(c echo.Context) error {
	dev := c.QueryParam("dev")
	if dev == "" {
		return echox.NormalErrorResponse(c, http.StatusBadRequest, http.StatusBadRequest, "missing dev")
	}
	opt := &grequests.RequestOptions{
		Data: map[string]string{
			"DoorDevId": dev,
		},
	}
	_, err := instance.doRequest("POST", SrvUrl+UrlOpenDoor, opt)
	if err != nil {
		klog.Errorf("openDoor failed: %s", err.Error())
		return echox.NormalErrorResponse(c, http.StatusInternalServerError, http.StatusInternalServerError, "openDoor failed")
	}
	return echox.NormalEmptyResponse(c)
}
