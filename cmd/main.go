package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/davidleitw/baha/internal/craw"
	"github.com/davidleitw/baha/internal/db"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logrus.SetReportCaller(true)
}

func printEachPageInfo(buildingRecord *db.BuildingRecord) {
	for _, pageRecord := range buildingRecord.Pages {
		prettyJsonStr, err := json.MarshalIndent(pageRecord, "", "  ")
		if err != nil {
			logrus.WithError(err).Error("json.MarshalIndent error")
			return
		}
		fmt.Printf("%s\n", prettyJsonStr)
	}
}

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Fatalf("Error loading .env file: %v", err)
	}

	account := os.Getenv("ACCOUNT")
	password := os.Getenv("PASSWORD")

	crawler, err := craw.NewCrawler()
	if err != nil {
		logrus.WithError(err).Error("NewCrawler error")
		return
	}

	crawler.LoadAuthCookies(account, password)

	url := "https://forum.gamer.com.tw/C.php?bsn=60076&snA=8294384&tnum=4"
	buildingRecord, err := crawler.ScrapingBuildingWithUrl(url)
	if err != nil {
		logrus.WithError(err).Error("ScrapingBuilding error")
		return
	}
	printEachPageInfo(buildingRecord)
}
