package main

import (
	"os"
	"time"

	"github.com/davidleitw/baha/internal/monitor"
	"github.com/davidleitw/baha/internal/rule"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

const (
	CsBuildingNo = 3146926
)

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logrus.SetReportCaller(true)
}

func newTrackRule(Id string) *rule.TrackingRule {
	return rule.NewTrackingRule(
		rule.Bsn(60076),
		rule.Sna(CsBuildingNo),
		rule.Id(Id),
		rule.PokeInterval(10*time.Second),
	)
}

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Fatalf("Error loading .env file: %v", err)
	}

	account := os.Getenv("ACCOUNT")
	password := os.Getenv("PASSWORD")

	rules := []*rule.TrackingRule{
		newTrackRule("leichitw"),
	}

	monitor, err := monitor.NewMonitor(
		account,
		password,
		rules...,
	)
	if err != nil {
		logrus.WithError(err).Error("monitor.NewMonitor failed")
		return
	}

	if err := monitor.Run(); err != nil {
		logrus.WithError(err).Error("monitor.Run failed")
	}
}
