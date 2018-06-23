package main

import (
	"errors"
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
	if _, err := os.Lstat(analylizer.cityDataDir); err != nil {
		return errors.New("未找到城市数据")
	}
	modelCount := analylizer.analysisCurrentOrder()
	modelScore := analylizer.analysisRepurchase()
	topn := c.Int("top")
	analylizer.Output(modelCount, modelScore, topn)
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

func (ca *CityAnalyzer) analysisCurrentOrder() map[string]int {
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

	return modelCount

}

type CarModelScore struct {
	Model string
	Score int
}

func (ca *CityAnalyzer) analysisRepurchase() map[string]int {
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

	return modelScore
}

func (ca *CityAnalyzer) Output(modelCount, modelScore map[string]int, topn int) {

	carModelCountList := []CarModelCount{}
	for model, count := range modelCount {
		carModelCountList = append(carModelCountList, CarModelCount{
			Model: model,
			Count: count,
		})
	}

	sort.Slice(carModelCountList, func(i, j int) bool { return carModelCountList[i].Count > carModelCountList[j].Count })

	log.Notice("\n车型订单数量排名:")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"排名", "车型", "实时订单数"})
	for i, mc := range carModelCountList {
		if i >= topn {
			break
		}
		table.Append([]string{
			fmt.Sprint(i + 1),
			mc.Model,
			fmt.Sprint(mc.Count),
		})
	}
	table.Render()

	carModelScoreList := []CarModelScore{}
	for model, score := range modelScore {
		carModelScoreList = append(carModelScoreList, CarModelScore{
			Model: model,
			Score: score,
		})
	}

	sort.Slice(carModelScoreList, func(i, j int) bool { return carModelScoreList[i].Score > carModelScoreList[j].Score })

	log.Notice("\n车型加油积分排名:")
	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"排名", "车型", "加油积分", "平均积分"})
	for i, ms := range carModelScoreList {
		if i >= topn {
			break
		}
		avgScore := ""
		if count, found := modelCount[ms.Model]; found && count != 0 {
			avgScore = fmt.Sprintf("%.02f", float64(ms.Score)/float64(count))
		} else {
			avgScore = "N/A"
		}
		table.Append([]string{
			fmt.Sprint(i + 1),
			ms.Model,
			fmt.Sprint(ms.Score),
			avgScore,
		})
	}
	table.Render()
}
