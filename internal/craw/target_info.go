package craw

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/sirupsen/logrus"
)

const (
	BahaBaseUrl = "https://forum.gamer.com.tw/C.php?"
)

type TargetInfo struct {
	Bsn  int
	Sna  int
	Page int
}

func (targetInfo *TargetInfo) selfValidate() error {
	if targetInfo == nil {
		return fmt.Errorf("targetInfo is nil")
	}

	if targetInfo.Bsn <= 0 || targetInfo.Sna <= 0 {
		return fmt.Errorf("targetInfo is invalid")
	}

	return nil
}

func (targetInfo *TargetInfo) GetBuildingUrl() string {
	return fmt.Sprintf("%sbsn=%d&snA=%d", BahaBaseUrl, targetInfo.Bsn, targetInfo.Sna)
}

func (targetInfo *TargetInfo) GetPageUrl(page int) string {
	return fmt.Sprintf("%sbsn=%d&snA=%d&page=%d", BahaBaseUrl, targetInfo.Bsn, targetInfo.Sna, page)
}

func GetTargetInfoFromUrl(rawURL string) (*TargetInfo, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		logrus.WithError(err).Error("url.Parse failed")
		return nil, err
	}

	targetInfo := &TargetInfo{}
	params := parsedURL.Query()
	if bsns := params.Get("bsn"); bsns != "" {
		bsn, err := strconv.Atoi(params.Get("bsn"))
		if err != nil {
			logrus.WithError(err).Error("strconv.Atoi failed")
			return nil, err
		}
		targetInfo.Bsn = bsn
	}

	if snas := params.Get("snA"); snas != "" {
		sna, err := strconv.Atoi(params.Get("snA"))
		if err != nil {
			logrus.WithError(err).Error("strconv.Atoi failed")
			return nil, err
		}
		targetInfo.Sna = sna
	}
	return targetInfo, nil
}
