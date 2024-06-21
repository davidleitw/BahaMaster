package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/davidleitw/baha/internal/baha"
	"github.com/davidleitw/baha/internal/craw"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
}

func printEachPageInfo(buildingRecord *baha.BuildingRecord) {
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

	crawler := craw.NewCrawler()
	crawler.LoadAuthCookies(&craw.UserInfo{
		Account:  account,
		Password: password,
	})
	buildingRecord, err := crawler.ScrapingBuilding(&craw.TargetInfo{
		Bsn: 60076,
		Sna: 8292214,
	})
	if err != nil {
		logrus.WithError(err).Error("ScrapingBuilding error")
		return
	}
	printEachPageInfo(buildingRecord)
}
