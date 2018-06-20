package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/olekukonko/tablewriter"

	"github.com/liudanking/goutil/encodingutil"
	log "github.com/liudanking/goutil/logutil"
	"github.com/urfave/cli"
)

func analysisCity(c *cli.Context) error {
	dir := c.String("dir")
	city := c.String("city")
	analylizer := NewCityAnalyzer(dir, city)
	analylizer.analysisCurrentOrder()
	analylizer.analysisRepurchase()
	return nil
}

type CityAnalyzer struct {
	cityName    string
	cityDataDir string
}

func NewCityAnalyzer(dir, city string) *CityAnalyzer {
	return &CityAnalyzer{
		cityName:    city,
		cityDataDir: filepath.Join(dir, city),
	}
}

type CarModelCount struct {
	Model string
	Count int
}

// type CarModelCountList []CarModelCount

// func (a CarModelCountList) Len() int           { return len(a) }
// func (a CarModelCountList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
// func (a CarModelCountList) Less(i, j int) bool { return a[i].Count < a[j].Count }

func (ca *CityAnalyzer) analysisCurrentOrder() {
	currentOrderDir := filepath.Join(ca.cityDataDir, "currentorder")

	modelCount := map[string]int{}
	filepath.Walk(currentOrderDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		items := map[string]CurrentOrderItem{}
		if err := encodingutil.UnmarshalJSONFromFile(path, &items); err != nil {
			log.Warning("unmarshal from %s failed:%v", path, err)
			return nil
		}

		for _, item := range items {
			if item.CarModel != "" {
				modelCount[item.CarModel]++
			}
		}

		return nil
	})

	carModelCountList := []CarModelCount{}
	for model, count := range modelCount {
		carModelCountList = append(carModelCountList, CarModelCount{
			Model: model,
			Count: count,
		})
	}

	sort.Slice(carModelCountList, func(i, j int) bool { return carModelCountList[i].Count > carModelCountList[j].Count })

	log.Notice("car rank:")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"#", "车型", "实时订单数量"})
	for i, mc := range carModelCountList {
		if i > 20 {
			break
		}
		table.Append([]string{
			fmt.Sprint(i),
			mc.Model,
			fmt.Sprint(mc.Count),
		})
	}

	table.Render()

}

type CarModelScore struct {
	Model string
	Score int
}

func (ca *CityAnalyzer) analysisRepurchase() {
	repurchaseDir := filepath.Join(ca.cityDataDir, "repurchase")

	modelScore := map[string]int{}
	filepath.Walk(repurchaseDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		items := map[string]RepurchaseItem{}
		if err := encodingutil.UnmarshalJSONFromFile(path, &items); err != nil {
			log.Warning("unmarshal from %s failed:%v", path, err)
			return nil
		}

		for _, item := range items {
			if item.CarModel != "" {
				modelScore[item.CarModel] += item.OrderCount1M
			}
		}

		return nil
	})

	carModelScoreList := []CarModelScore{}
	for model, score := range modelScore {
		carModelScoreList = append(carModelScoreList, CarModelScore{
			Model: model,
			Score: score,
		})
	}

	sort.Slice(carModelScoreList, func(i, j int) bool { return carModelScoreList[i].Score > carModelScoreList[j].Score })

	log.Notice("car score rank:")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"#", "车型", "加油积分"})
	for i, ms := range carModelScoreList {
		if i > 20 {
			break
		}
		table.Append([]string{
			fmt.Sprint(i),
			ms.Model,
			fmt.Sprint(ms.Score),
		})
	}
	table.Render()

}
