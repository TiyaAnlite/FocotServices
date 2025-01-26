package main

import (
	"github.com/TiyaAnlite/FocotServicesCommon/echox"
	"github.com/labstack/echo/v4"
	"k8s.io/klog/v2"
	"net/http"
	"strconv"
)

type StaticConfigProvider struct {
	Rooms []string `json:"rooms" yaml:"rooms"`
}

func (p *StaticConfigProvider) Init(*CenterContext) error {
	return nil
}

func (p *StaticConfigProvider) Provide(c chan<- *ProvidedRoom) {
	for _, room := range p.Rooms {
		roomId, err := strconv.ParseUint(room, 10, 64)
		if err != nil {
			klog.Errorf("Failed to parse room id from config: %s", room)
			continue
		}
		c <- &ProvidedRoom{
			ProviderName: "static",
			RoomID:       roomId,
		}
	}
}

func (p *StaticConfigProvider) Revoke(chan<- uint64) {

}

type ApiConfigProvider struct {
	Path string `json:"path" yaml:"path"`
	e    *echo.Echo
}

func (p *ApiConfigProvider) Init(c *CenterContext) error {
	klog.Infof("[ApiConfigProvider]init path at: %s", p.Path)
	p.e = c.Echo
	return nil
}

func (p *ApiConfigProvider) Provide(r chan<- *ProvidedRoom) {
	p.e.GET(p.Path+"/:roomId", func(c echo.Context) error {
		room := c.Param("roomId")
		roomId, err := strconv.ParseUint(room, 10, 64)
		if err != nil {
			return echox.NormalErrorResponse(c, http.StatusBadRequest, http.StatusBadRequest, err.Error())
		}
		r <- &ProvidedRoom{
			ProviderName: "api",
			RoomID:       roomId,
		}
		return echox.NormalResponse(c, http.StatusOK)
	})
}

func (p *ApiConfigProvider) Revoke(r chan<- uint64) {
	p.e.DELETE(p.Path+"/:roomId", func(c echo.Context) error {
		room := c.Param("roomId")
		roomId, err := strconv.ParseUint(room, 10, 64)
		if err != nil {
			return echox.NormalErrorResponse(c, http.StatusBadRequest, http.StatusBadRequest, err.Error())
		}
		r <- roomId
		return echox.NormalResponse(c, http.StatusOK)
	})
}
