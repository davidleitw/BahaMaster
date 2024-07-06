package main

import (
	"fmt"
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

	url := "https://forum.gamer.com.tw/C.php?page=38935&bsn=60076&snA=5170434"
	pageRecord, err := crawler.ParsePage(url)
	if err != nil {
		logrus.WithError(err).Error("ParsePage error")
		return
	}
	logrus.Info("size of floors: ", len(pageRecord.Floors))
	for _, floor := range pageRecord.Floors {
		fmt.Printf("Index = %d, Author: %s, Content: %s\n", floor.FloorIndex, floor.AuthorName, floor.Content)
		for _, reply := range floor.Replies {
			fmt.Printf(" > Reply: %s\n", reply.Content)
		}
		fmt.Println()
	}
}
