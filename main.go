package main

import (
	"os"

	log "github.com/liudanking/goutil/logutil"

	"github.com/urfave/cli"
)

func main() {

	app := cli.NewApp()
	app.Version = "0.0.1"
	app.Usage = "Collect didi gas station data, and rank most popular didi cars"
	app.EnableBashCompletion = true
	app.Commands = []cli.Command{
		cli.Command{
			Name:  "collect_data",
			Usage: "Collect didi gas station data",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "listen, l",
					Usage: "listen addr",
					Value: ":8086",
				},
				cli.StringFlag{
					Name:  "dir, d",
					Usage: "directory for saving data",
					Value: "./data",
				},
			},
			Action: collectData,
		},
		cli.Command{
			Name:  "analysis",
			Usage: "Analysis collected data and output most popular didi cars",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dir, d",
					Usage: "data directory",
					Value: "./data",
				},
				cli.StringFlag{
					Name:  "city, c",
					Usage: "city name",
					Value: "成都市",
				},
			},
			Action: analysisCity,
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Error(err.Error())
		return
	}
}
