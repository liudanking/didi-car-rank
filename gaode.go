package main

import (
	"errors"

	"fmt"

	log "github.com/liudanking/goutil/logutil"
	"github.com/liudanking/goutil/netutil"
)

const (
	// Oops, for temporary usage
	GaodeKey = "64b4b2d0050ae7a675ec7b2cd2a5976c"
)

type RegeoRsp struct {
	Status    string `json:"status"`
	Info      string `json:"info"`
	Infocode  string `json:"infocode"`
	Regeocode struct {
		AddressComponent struct {
			Country  string      `json:"country"`
			Province string      `json:"province"`
			City     interface{} `json:"city"`
		} `json:"addressComponent"`
	} `json:"regeocode"`
}

func (rsp *RegeoRsp) GetCity() string {
	city, ok := rsp.Regeocode.AddressComponent.City.(string)
	if ok && city != "" {
		return city
	}
	// 直辖市
	if rsp.Regeocode.AddressComponent.Province != "" {
		return rsp.Regeocode.AddressComponent.Province
	}
	return "未知"
}

func GetRegeoInfo(lng string, lat string) (*RegeoRsp, error) {
	addr := "http://restapi.amap.com/v3/geocode/regeo"
	params := map[string]interface{}{
		"key":      GaodeKey,
		"location": fmt.Sprintf("%s,%s", lng, lat),
	}

	rsp := &RegeoRsp{}
	_, err := netutil.DefaultHttpClient().RequestForm("GET", addr, params).DoJSON(rsp)
	if err != nil {
		return nil, err
	}
	if rsp.Status == "0" {
		return nil, errors.New(rsp.Info)
	}

	return rsp, nil

}

func GetCityByPosition(lng, lat string) string {
	rsp, err := GetRegeoInfo(lng, lat)
	if err != nil {
		log.Warning("get regeo [%s, %s] failed:%v", lng, lat, err)
		return "未知"
	}
	return rsp.GetCity()
}
