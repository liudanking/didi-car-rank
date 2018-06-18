package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liudanking/goutil/encodingutil"

	"github.com/liudanking/goutil/netutil"

	log "github.com/liudanking/goutil/logutil"

	"github.com/elazarl/goproxy"
	"github.com/urfave/cli"
)

func collectData(c *cli.Context) error {
	if err := setCA(caCert, caKey); err != nil {
		log.Error("setCA failed:%v", err)
	}
	proxy := goproxy.NewProxyHttpServer()
	// proxy.Verbose = true

	listenAddr := c.String("listen")
	if listenAddr == "" {
		return errors.New("listen address is empty")
	}
	dh := NewDidiHooker(c.String("dir"))
	dh.RegisterHook(proxy)

	log.Info("start serving %s", listenAddr)
	// go func() {
	if err := http.ListenAndServe(listenAddr, proxy); err != nil {
		log.Error("listen %s failed:%v", listenAddr, err)
		os.Exit(1)
	}
	// }()
	return nil
}

type DidiHooker struct {
	dataMtx  sync.Mutex
	dataDir  string
	lastCity string
}

func NewDidiHooker(dataDir string) *DidiHooker {
	return &DidiHooker{
		dataDir:  dataDir,
		lastCity: "未知",
	}
}

func (dh *DidiHooker) RegisterHook(p *goproxy.ProxyHttpServer) {
	dstHost := "devcon-go.am.xiaojukeji.com:443"
	p.OnRequest(goproxy.DstHostIs(dstHost)).HandleConnect(goproxy.AlwaysMitm)
	p.OnResponse(goproxy.DstHostIs(dstHost)).DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {

		if strings.HasPrefix(ctx.Req.URL.Path, "/front/gasstation/index") {
			log.Info("gasstation hook!")
			return dh.hookGasstation(resp, ctx)
		} else if strings.HasPrefix(ctx.Req.URL.Path, "/map/store/near") {
			log.Info("near store hook!")
			return dh.hookNearStore(resp, ctx)
		}

		return resp
	})
}

func (dh *DidiHooker) hookGasstation(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	data, err := repeatReadBody(resp)
	if err != nil {
		log.Error("read gasstation rsp failed:%v", err)
		return resp
	}

	s := string(data)

	startStr := `$CONFIG = JSON.parse(`
	start := strings.Index(s, startStr)
	if start < 0 {
		log.Warning("gasstation data start index not found")
		return resp
	}
	end := strings.Index(s[start:], ");\n")
	if end < 0 {
		log.Warning("gasstation data end index not found")
		return resp
	}

	subs := s[start+len(startStr) : start+end]
	gasstationStr, err := strconv.Unquote(subs)
	if err != nil {
		log.Warning("unquote [%s] failed:%v", subs, err)
		return resp
	}

	rsp := &ListGasstationRsp{}
	if err := json.Unmarshal([]byte(gasstationStr), rsp); err != nil {
		log.Error("unmarshal [%s] failed:%v", gasstationStr, err)
		return resp
	}
	dh.lastCity = rsp.CityName

	dh.dataMtx.Lock()
	if err := rsp.updateToFile(dh.cityDataDir()); err != nil {
		log.Error("update gasstation data failed:%v", err)
	}
	dh.dataMtx.Unlock()

	go func() {
		dh.dataMtx.Lock()
		defer dh.dataMtx.Unlock()
		dh.doCollectData(rsp.StoreForMap, rsp.AmChannel)
	}()

	return resp

}

type NearStoreRsp struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   struct {
		StoreCount            int           `json:"store_count"`
		StoreType             int           `json:"store_type"`
		StoreForMap           []Store       `json:"store_for_map"`
		StoreList             []interface{} `json:"store_list"`
		FilterCondition       interface{}   `json:"filter_condition"`
		SelectedFuelCategory  string        `json:"selected_fuel_category"`
		SelectedGoodsCategory string        `json:"selected_goods_category"`
		SelectedBrand         string        `json:"selected_brand"`
		FuelCategoryName      string        `json:"fuel_category_name"`
		GoodsCategoryName     string        `json:"goods_category_name"`
		BrandName             string        `json:"brand_name"`
		TotalScore            string        `json:"total_score"`
	} `json:"data"`
}

func (dh *DidiHooker) hookNearStore(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	data, err := repeatReadBody(resp)
	if err != nil {
		log.Warning("read near store body failed:%v", err)
		return resp
	}

	rsp := &NearStoreRsp{}
	if err := json.Unmarshal(data, rsp); err != nil {
		log.Warning("unmarshal near store rsp failed:%v", err)
		return resp
	}

	// TODO: get city by lat,long
	go func() {
		dh.dataMtx.Lock()
		defer dh.dataMtx.Unlock()
		dh.doCollectData(rsp.Data.StoreForMap, 10001)
	}()

	return resp

}

func (dh *DidiHooker) cityDataDir() string {
	return filepath.Join(dh.dataDir, dh.lastCity)
}

func (dh *DidiHooker) doCollectData(stores []Store, amChannel int) {
	dir := dh.cityDataDir()
	currentOrderDir := filepath.Join(dir, "currentorder")
	repurchaseDir := filepath.Join(dir, "repurchase")
	os.MkdirAll(currentOrderDir, 0700)
	os.MkdirAll(repurchaseDir, 0700)

	// current order
	for _, store := range stores {
		fn := filepath.Join(currentOrderDir, fmt.Sprintf("%s.json", store.StoreID))
		fi, err := os.Lstat(fn)
		v := map[string]CurrentOrderItem{}
		if err == nil {
			if time.Since(fi.ModTime()) < 5*time.Second {
				continue
			} else {
				if err := encodingutil.UnmarshalJSONFromFile(fn, &v); err != nil {
					log.Warning("unmarshal from file %s failed:%v", err)
				}
			}
		}

		currentOrderRsp, err := store.GetCurrentOrder(amChannel)
		if err != nil {
			log.Warning("get [store_id:%s] current order failed:%v", store.StoreID, err)
			continue
		}
		for _, item := range currentOrderRsp.Data.Items {
			v[item.ID] = item
		}

		if err := jsonMarshalIndentToFile(fn, &v); err != nil {
			log.Warning("write json data to %s failed:%v", err)
		}

	}
	files, _ := ioutil.ReadDir(currentOrderDir)
	log.Info("saved %d store currentorder data", len(files))

	// repurchase
	for _, store := range stores {
		fn := filepath.Join(repurchaseDir, fmt.Sprintf("%s.json", store.StoreID))
		fi, err := os.Lstat(fn)
		v := map[string]RepurchaseItem{}
		if err == nil {
			if time.Since(fi.ModTime()) < 5*time.Second {
				continue
			} else {
				if err := encodingutil.UnmarshalJSONFromFile(fn, &v); err != nil {
					log.Warning("unmarshal from file %s failed:%v", err)
				}
			}
		}

		repurchaseDriverRsp, err := store.GetRepurchaseDriver(amChannel)
		if err != nil {
			log.Warning("get [store_id:%s] current order failed:%v", store.StoreID, err)
			continue
		}
		for _, item := range repurchaseDriverRsp.Data.Items {
			v[item.DriverID] = item
		}

		if err := jsonMarshalIndentToFile(fn, &v); err != nil {
			log.Warning("write json data to %s failed:%v", err)
		}

	}
	files, _ = ioutil.ReadDir(repurchaseDir)
	log.Info("saved %d store repurchase data", len(files))
}

type ListGasstationRsp struct {
	AmChannel     int    `json:"am_channel"`
	Avater        string `json:"avater"`
	BrandName     string `json:"brand_name"`
	CenterURL     string `json:"center_url"`
	CityID        string `json:"city_id"`
	CityName      string `json:"city_name"`
	ConfirmURL    string `json:"confirm_url"`
	CouponCount   int    `json:"coupon_count"`
	DistanceCount struct {
		Num8000 int `json:"8000"`
	} `json:"distance_count"`
	FilterCondition struct {
		Oil struct {
			FuelCategoryInfo struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"fuel_category_info"`
			GoodsCategoryInfo []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"goods_category_info"`
			BrandInfo []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"brand_info"`
		} `json:"oil"`
		Gas struct {
			FuelCategoryInfo struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"fuel_category_info"`
			GoodsCategoryInfo []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"goods_category_info"`
			BrandInfo []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"brand_info"`
		} `json:"gas"`
	} `json:"filter_condition"`
	FuelCategoryName      string  `json:"fuel_category_name"`
	GoodsCategoryName     string  `json:"goods_category_name"`
	GulfstreamCityID      int     `json:"gulfstream_city_id"`
	Lat                   float64 `json:"lat"`
	Lng                   float64 `json:"lng"`
	PassportUID           string  `json:"passport_uid"`
	Phone                 string  `json:"phone"`
	SelectedBrand         string  `json:"selected_brand"`
	SelectedFuelCategory  string  `json:"selected_fuel_category"`
	SelectedGoodsCategory string  `json:"selected_goods_category"`
	StoreCount            int     `json:"store_count"`
	StoreForMap           []Store `json:"store_for_map"`
	StoreList             []struct {
		StoreID            string        `json:"store_id"`
		Name               string        `json:"name"`
		Logo               string        `json:"logo"`
		LogoX              string        `json:"logo_x"`
		LogoXx             string        `json:"logo_xx"`
		Lat                float64       `json:"lat"`
		Lng                float64       `json:"lng"`
		Price              string        `json:"price"`
		Discount           string        `json:"discount"`
		Distance           string        `json:"distance"`
		Address            string        `json:"address"`
		MonthOrderCount    string        `json:"month_order_count"`
		RepurchaseUserRate int           `json:"repurchase_user_rate"`
		Rank               int           `json:"rank"`
		RankText           string        `json:"rank_text"`
		IsNew              int           `json:"is_new"`
		Rawid              string        `json:"rawid"`
		ActivityNum        int           `json:"activity_num"`
		ActivityList       []interface{} `json:"activity_list"`
		CouponInfo         interface{}   `json:"coupon_info"`
		PromotionContent   string        `json:"promotion_content"`
		FreshUser          int           `json:"fresh_user"`
		TotalScore         string        `json:"total_score"`
		DidiGuideDiscount  string        `json:"didi_guide_discount"`
		RankDidiDiscount   string        `json:"rank_didi_discount"`
		RankGuideDiscount  string        `json:"rank_guide_discount"`
		RankStoreDiscount  string        `json:"rank_store_discount"`
		RankPrice          string        `json:"rank_price"`
	} `json:"store_list"`
	StoreType    int    `json:"store_type"`
	Ticket       string `json:"ticket"`
	UserRank     int    `json:"user_rank"`
	UserRankImg  string `json:"user_rank_img"`
	UserRankName string `json:"user_rank_name"`
}

type Store struct {
	StoreID  string  `json:"store_id"`
	Name     string  `json:"name"`
	Logo     string  `json:"logo"`
	LogoX    string  `json:"logo_x"`
	LogoXx   string  `json:"logo_xx"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Rawid    string  `json:"rawid"`
	Distance string  `json:"distance"`
	Price    string  `json:"price"`
}

// func (rsp *ListGasstationRsp) cityDataDir(dir string) string {
// 	return filepath.Join(dir, rsp.CityName)
// }

func (rsp *ListGasstationRsp) updateToFile(dir string) error {
	// dir = rsp.cityDataDir(dir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	fn := filepath.Join(dir, "gasstations.json")

	v := map[string]Store{}
	if _, err := os.Lstat(fn); err == nil {
		if err := encodingutil.UnmarshalJSONFromFile(fn, &v); err != nil {
			log.Warning("unmarshal failed:%v", err)
			return err
		}
	}

	for _, store := range rsp.StoreForMap {
		v[store.StoreID] = store
	}

	return jsonMarshalIndentToFile(fn, &v)

}

type CurrentOrderRsp struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   struct {
		Page  int                `json:"page"`
		Size  int                `json:"size"`
		Total int                `json:"total"`
		Items []CurrentOrderItem `json:"items"`
	} `json:"data"`
}

type CurrentOrderItem struct {
	ID           string `json:"id"`
	UID          string `json:"uid"`
	Pid          string `json:"pid"`
	UserName     string `json:"user_name"`
	Avater       string `json:"avater"`
	SalePrice    string `json:"sale_price"`
	RealPrice    string `json:"real_price"`
	RealPriceFmt string `json:"real_price_fmt"`
	Status       int    `json:"status"`
	PayTime      int    `json:"pay_time"`
	PayTimeFmt   string `json:"pay_time_fmt"`
	CarModel     string `json:"car_model"`
	SavePrice    string `json:"save_price"`
	SavePriceFmt string `json:"save_price_fmt"`
}

func (store Store) GetCurrentOrder(amChannel int) (*CurrentOrderRsp, error) {
	addr := "https://devcon-go.am.xiaojukeji.com/front/statistic/currentorder"

	rsp := &CurrentOrderRsp{}
	data, err := netutil.DefaultHttpClient().UserAgent(netutil.UA_CHROME).
		RequestForm("GET", addr, map[string]interface{}{
			"am_channel": amChannel,
			"store_id":   store.StoreID,
		}).DoJSON(rsp)
	if err != nil {
		log.Error("get currentorder [store_id:%s] failed:[data:%s]%v", store.StoreID, data, err)
		return nil, err
	}

	return rsp, nil
}

type RepurchaseDriverRsp struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   struct {
		Page  int              `json:"page"`
		Size  int              `json:"size"`
		Total int              `json:"total"`
		Items []RepurchaseItem `json:"items"`
	} `json:"data"`
}

type RepurchaseItem struct {
	DriverID           string `json:"driver_id"`
	Avanter            string `json:"avanter"`
	DriverName         string `json:"driver_name"`
	UserName           string `json:"user_name"`
	CarModel           string `json:"car_model"`
	OrderCount1M       int    `json:"order_count_1m"`
	Ordercount1M       int    `json:"ordercount_1m"`
	OrderDiscount1MFmt string `json:"order_discount_1m_fmt"`
}

func (store Store) GetRepurchaseDriver(amChannel int) (*RepurchaseDriverRsp, error) {
	addr := "https://devcon-go.am.xiaojukeji.com/front/statistic/repurchase"
	rsp := &RepurchaseDriverRsp{}
	data, err := netutil.DefaultHttpClient().UserAgent(netutil.UA_CHROME).
		RequestForm("GET", addr, map[string]interface{}{
			"am_channel": amChannel,
			"store_id":   store.StoreID,
		}).DoJSON(rsp)
	if err != nil {
		log.Error("get currentorder [store_id:%s] failed:[data:%s]%v", store.StoreID, data, err)
		return nil, err
	}

	return rsp, nil
}

// func (rsp *ListGasstationRsp) doCollectData(dataDir string) {
// 	dir := rsp.cityDataDir(dataDir)
// 	currentOrderDir := filepath.Join(dir, "currentorder")
// 	repurchaseDir := filepath.Join(dir, "repurchase")
// 	os.MkdirAll(currentOrderDir, 0700)
// 	os.MkdirAll(repurchaseDir, 0700)

// 	// current order
// 	for _, store := range rsp.StoreForMap {
// 		fn := filepath.Join(currentOrderDir, fmt.Sprintf("%s.json", store.StoreID))
// 		fi, err := os.Lstat(fn)
// 		v := map[string]CurrentOrderItem{}
// 		if err == nil {
// 			if time.Since(fi.ModTime()) < 5*time.Second {
// 				continue
// 			} else {
// 				if err := encodingutil.UnmarshalJSONFromFile(fn, &v); err != nil {
// 					log.Warning("unmarshal from file %s failed:%v", err)
// 				}
// 			}
// 		}

// 		currentOrderRsp, err := store.GetCurrentOrder(rsp.AmChannel)
// 		if err != nil {
// 			log.Warning("get [store_id:%s] current order failed:%v", store.StoreID, err)
// 			continue
// 		}
// 		for _, item := range currentOrderRsp.Data.Items {
// 			v[item.ID] = item
// 		}

// 		if err := jsonMarshalIndentToFile(fn, &v); err != nil {
// 			log.Warning("write json data to %s failed:%v", err)
// 		}

// 		files, _ := ioutil.ReadDir(currentOrderDir)
// 	}
// 	log.Info("saved %d store currentorder data", len(files))

// 	// repurchase
// 	for _, store := range rsp.StoreForMap {
// 		fn := filepath.Join(repurchaseDir, fmt.Sprintf("%s.json", store.StoreID))
// 		fi, err := os.Lstat(fn)
// 		v := map[string]RepurchaseItem{}
// 		if err == nil {
// 			if time.Since(fi.ModTime()) < 5*time.Second {
// 				continue
// 			} else {
// 				if err := encodingutil.UnmarshalJSONFromFile(fn, &v); err != nil {
// 					log.Warning("unmarshal from file %s failed:%v", err)
// 				}
// 			}
// 		}

// 		repurchaseDriverRsp, err := store.GetRepurchaseDriver(rsp.AmChannel)
// 		if err != nil {
// 			log.Warning("get [store_id:%s] current order failed:%v", store.StoreID, err)
// 			continue
// 		}
// 		for _, item := range repurchaseDriverRsp.Data.Items {
// 			v[item.DriverID] = item
// 		}

// 		if err := jsonMarshalIndentToFile(fn, &v); err != nil {
// 			log.Warning("write json data to %s failed:%v", err)
// 		}

// 		files, _ := ioutil.ReadDir(repurchaseDir)
// 	}
// 	log.Info("saved %d store repurchase data", len(files))
// }
