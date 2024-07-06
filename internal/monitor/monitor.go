package monitor

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidleitw/baha/internal/craw"
	"github.com/davidleitw/baha/internal/rule"
	"github.com/sirupsen/logrus"
)

type Monitor interface {
	Run() error
}

type monitor struct {
	crawler craw.Crawler

	rules  []*rule.TrackingRule
	stopCh chan struct{}
}

var _ Monitor = &monitor{}

func NewMonitor(account, password string, rules ...*rule.TrackingRule) (Monitor, error) {
	crawler, err := craw.NewCrawler()
	if err != nil {
		logrus.WithError(err).Error("NewCrawler error")
		return nil, err
	}

	if err := crawler.LoginAndKeepCookies(account, password); err != nil {
		logrus.WithError(err).Error("LoginAndKeepCookies error")
		return nil, err
	}

	return &monitor{rules: rules, crawler: crawler, stopCh: make(chan struct{})}, nil
}

func (m *monitor) activateTrackLoop(rule *rule.TrackingRule) {
	firstTimeFlag := true
	maxFailure := rule.GetMaxFailure()
	interval := rule.GetInterval()

	for {
		select {
		case <-m.stopCh:
			logrus.Info("Stop aim loop")
			return
		default:
			pageRecord, err := m.crawler.ParsePage(rule.LastPageUrl)
			if err != nil || pageRecord.Floors == nil {
				logrus.WithError(err).Error("ParsePage error")

				maxFailure--
				if maxFailure == 0 {
					logrus.Error("Max failure reached")
					m.stopCh <- struct{}{}
				}

				time.Sleep(interval)
				continue
			}

			lastFloor := pageRecord.Floors[len(pageRecord.Floors)-1]
			// Mean new floor
			if !firstTimeFlag && lastFloor.FloorIndex != rule.LastFloorIndex {
				rule.NewPostCallback(lastFloor)
			}

			// Mean update content
			if !firstTimeFlag && lastFloor.Content != rule.LastFloorRecord.Content {
				rule.UpdateLastCallback(lastFloor)
			}

			rule.LastFloorIndex = lastFloor.FloorIndex
			rule.LastFloorRecord = lastFloor
			firstTimeFlag = false
			time.Sleep(interval)
		}
	}
}

func (m *monitor) Run() error {
	for _, rule := range m.rules {
		go m.activateTrackLoop(rule)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	close(m.stopCh)

	logrus.Info("Shutting down monitor ...")
	time.Sleep(1 * time.Second)
	return nil
}
