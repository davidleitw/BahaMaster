package main

import (
	"os"

	"github.com/davidleitw/baha/internal/craw"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logrus.SetReportCaller(true)
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

	if err := crawler.LoginAndKeepCookies(account, password); err != nil {
		logrus.WithError(err).Error("LoginAndKeepCookies error")
		return
	}

	url := "https://forum.gamer.com.tw/C.php?bsn=60076&snA=8294384&tnum=4"
	if err := crawler.ScrapingBuildingWithUrl(url); err != nil {
		logrus.WithError(err).Error("ScrapingBuilding error")
		return
	}
}
