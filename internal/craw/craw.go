package craw

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davidleitw/baha/internal/db"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	BahaBaseUrl = "https://forum.gamer.com.tw/C.php?"
	// LoginURLPhase1 is the first URL to login
	// Example: https://user.gamer.com.tw/login.php
	// This URL is used to get the alternativeCaptcha value
	LoginURLPhase1 = "https://user.gamer.com.tw/login.php"

	// LoginURLPhase2 is the second URL to login
	// Example: https://user.gamer.com.tw/ajax/do_login.php
	// This URL is used to login with the alternativeCaptcha value and set the cookies
	LoginURLPhase2 = "https://user.gamer.com.tw/ajax/do_login.php"

	// ExtendReplyURL is the URL to get more replies
	// Example: https://forum.gamer.com.tw/ajax/moreCommend.php?bsn=60076&snB=8292214&returnHtml=0
	ExtendReplyURL = "https://forum.gamer.com.tw/ajax/moreCommend.php?"

	scrapingInterval = 1 * time.Second
)

type TargetInfo struct {
	Bsn  int
	Sna  int
	Page int
}

func (targetInfo *TargetInfo) validate() error {
	if targetInfo == nil {
		return fmt.Errorf("targetInfo is nil")
	}

	if targetInfo.Bsn <= 0 || targetInfo.Sna <= 0 {
		return fmt.Errorf("targetInfo is invalid")
	}

	return nil
}

func (targetInfo TargetInfo) GetBuildingUrl() string {
	return fmt.Sprintf("%sbsn=%d&snA=%d", BahaBaseUrl, targetInfo.Bsn, targetInfo.Sna)
}

func (targetInfo TargetInfo) GetPageUrl(page int) string {
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

type Crawler interface {
	getDocumentFromUrl(url string) (*goquery.Document, error)

	GetBuildingPageAndTitle(targetInfo *TargetInfo) (int, string, error)

	scrapingPage(pageUrl string, pageIndex int, buildingId string) error

	ScrapingBuilding(targetInfo *TargetInfo) error

	ScrapingBuildingWithUrl(url string) error

	LoginAndKeepCookies(account, password string) error
}

type crawler struct {
	isSessionActive bool

	client *resty.Client
	db     db.BuildingDB
}

func NewCrawler() (Crawler, error) {
	db := db.NewBuildingDb()
	if err := db.Open(); err != nil {
		logrus.WithError(err).Error("db.Open failed")
		return nil, err
	}

	return &crawler{
		isSessionActive: false,
		client:          resty.New(),
		db:              db,
	}, nil
}

var _ Crawler = (*crawler)(nil)

func (crawler *crawler) getDocumentFromUrl(url string) (*goquery.Document, error) {
	if !crawler.isSessionActive {
		logrus.Error("Session is not active, please use LoadAuthCookies to login")
		return nil, fmt.Errorf("session is not active")
	}

	res, err := crawler.client.R().Get(url)
	if err != nil {
		logrus.WithError(err).Errorf("GET %s failed", url)
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(res.Body()))
	if err != nil {
		logrus.WithError(err).Errorf("goquery.NewDocumentFromReader failed")
		return nil, err
	}
	return doc, nil
}

func (crawler *crawler) GetBuildingPageAndTitle(targetInfo *TargetInfo) (int, string, error) {
	doc, err := crawler.getDocumentFromUrl(targetInfo.GetBuildingUrl())
	if err != nil {
		logrus.WithError(err).Error("crawler.getDocumentFromUrl failed")
		return 0, "", err
	}

	max, err := strconv.Atoi(doc.Find("p.BH-pagebtnA>a").Last().Text())
	if err != nil {
		logrus.WithError(err).Errorf("strconv.Atoi failed")
		return 0, "", err
	}

	title := doc.Find("div.c-post__header>h1.c-post__header__title").Text()
	if title == "" {
		logrus.Errorf("title not found")
		return 0, "", fmt.Errorf("title not found")
	}
	return max, title, nil
}

func getAuthorIdFromHref(href string) string {
	// Split the URL by "/"
	parts := strings.Split(href, "/")

	// Get the last part of the URL
	lastPart := parts[len(parts)-1]

	return lastPart
}

func extractExtendAPIParams(onclickValue string) (int, int, error) {
	re := regexp.MustCompile(`extendComment\((\d+),\s*(\d+)\);`)

	matches := re.FindStringSubmatch(onclickValue)
	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("no matches found")
	}

	num1, err1 := strconv.Atoi(matches[1])
	num2, err2 := strconv.Atoi(matches[2])

	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("failed to convert to integer")
	}

	return num1, num2, nil
}

func (crawler *crawler) syncRepliesExtended(s *goquery.Selection, fid string) error {
	onclickValue, exist := s.Find("div.nocontent>a,more-reply").Attr("onclick")
	if !exist {
		logrus.Errorf("extendSection.Find a.more-reply id not found")
		return nil
	}

	bsn, snb, err := extractExtendAPIParams(onclickValue)
	if err != nil {
		logrus.WithError(err).Errorf("getExtendRequestId failed")
		return err
	}

	extendUrl := fmt.Sprintf("%sbsn=%d&snB=%d&returnHtml=0", ExtendReplyURL, bsn, snb)
	res, err := crawler.client.R().Get(extendUrl)
	if err != nil {
		logrus.WithError(err).Errorf("GET %s failed", extendUrl)
		return err
	}

	replyRes := map[string]interface{}{}
	if err := json.Unmarshal(res.Body(), &replyRes); err != nil {
		logrus.WithError(err).Error("Failed to unmarshal JSON")
		return err
	}

	for index, reply := range replyRes {
		if index == "next_snC" {
			continue
		}

		replyIndex, err := strconv.Atoi(index)
		if err != nil {
			logrus.WithError(err).Errorf("strconv.Atoi %s failed", index)
			continue
		}
		switch r := reply.(type) {
		case map[string]interface{}:
			record := &db.ReplyRecord{
				Fid:        fid,
				ReplyIndex: replyIndex,
				AuthorName: r["nick"].(string),
				AuthorId:   r["userid"].(string),
				Content:    r["comment"].(string),
			}
			if err := crawler.db.SyncReplyRecord(record); err != nil {
				logrus.WithError(err).Error("crawler.db.SyncReplyRecord failed")
			}
		default:
			logrus.Errorf("Unexpected type: %T", r)
		}
	}
	return nil
}

func (crawler *crawler) syncReplies(s *goquery.Selection, fid string) error {
	s.Find("div.c-reply__item>div>div.reply-content").Each(func(i int, s *goquery.Selection) {
		contentUserSection := s.Find("a.reply-content__user")
		name := contentUserSection.Text()
		id, exist := contentUserSection.Attr("href")
		if !exist {
			logrus.Errorf("contentUserSection.Attr href not found")
			return
		}

		record := &db.ReplyRecord{
			Fid:        fid,
			ReplyIndex: i,
			AuthorName: name,
			AuthorId:   getAuthorIdFromHref(id),
			Content:    s.Find("article.c-article>span.comment_content").Text(),
		}

		if err := crawler.db.SyncReplyRecord(record); err != nil {
			logrus.WithError(err).Error("crawler.db.SyncReplyRecord failed")
		}
	})
	return nil
}

func (crawler *crawler) syncFloorRecord(bid, pid, authName, authId, content string, floorIndex int) (string, error) {
	var err error
	var floorRecord *db.FloorRecord

	floorRecord, err = crawler.db.GetFloorRecord(bid, floorIndex)
	if err != nil {
		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("crawler.db.GetFloorRecord failed")
			return "", err
		}
		floorRecord = &db.FloorRecord{
			Bid:        bid,
			Pid:        pid,
			Fid:        uuid.New().String(),
			AuthorName: authName,
			AuthorId:   authId,
			Content:    content,
			FloorIndex: floorIndex,
		}

		if err = crawler.db.CreateFloorRecord(floorRecord); err != nil {
			logrus.WithError(err).Error("crawler.db.CreateFloorRecord failed")
			return "", err
		}
		return floorRecord.Fid, nil
	}

	if floorRecord.Content != content {
		if err = crawler.db.UpdateFloorRecordContent(floorRecord.Fid, content); err != nil {
			logrus.WithError(err).Error("crawler.db.UpdateFloorRecordContent failed")
			return "", err
		}
	}

	return floorRecord.Fid, nil
}

func (crawler *crawler) parseFloorSelection(s *goquery.Selection, bid, pid string) error {
	// get floor id, if it contains "disable" then it's not a valid floor
	disableFloor, exist := s.Attr("id")
	if !exist || strings.Contains(disableFloor, "disable") {
		return nil
	}

	mainSection := s.Find("div.c-section__main")

	authorSection := mainSection.Find("div.c-post__header__author")
	floorIndex, exist := authorSection.Find("a.floor").Attr("data-floor")
	if !exist {
		logrus.Errorf("authorSection.Find a.floor data-floor not found")
		return errors.New("floorIndex not found")
	}

	index, err := strconv.Atoi(floorIndex)
	if err != nil {
		logrus.WithError(err).Errorf("strconv.Atoi %s failed", floorIndex)
		return err
	}

	authorName := authorSection.Find("a.username").Text()
	authorId := authorSection.Find("a.userid").Text()

	content, err := mainSection.Find("div.c-article__content").Html()
	if err != nil {
		logrus.WithError(err).Errorf("mainSection.Find div.c-article__content failed")
		return errors.New("content not found")
	}

	fid, err := crawler.syncFloorRecord(bid, pid, authorName, authorId, content, index)
	if err != nil {
		logrus.WithError(err).Error("crawler.syncFloorRecord failed")
		return err
	}

	replyContentSection := mainSection.Find("div.c-reply")
	// If there are more replies, we need to send another request to get the full content
	if replyContentSection.Find("div.nocontent").Length() != 0 {
		return crawler.syncRepliesExtended(replyContentSection, fid)
	}
	return crawler.syncReplies(replyContentSection, fid)
}

func (crawler *crawler) getPageRecord(buildingId string, pageIndex int) (*db.PageRecord, error) {
	var err error
	var pageRecord *db.PageRecord

	pageRecord, err = crawler.db.GetPageRecord(buildingId, pageIndex)
	if err != nil {
		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("crawler.db.GetPageRecord failed")
			return nil, err
		}

		pageRecord = &db.PageRecord{
			Pid:       uuid.New().String(),
			Bid:       buildingId,
			PageIndex: pageIndex,
		}

		if err = crawler.db.CreatePageRecord(pageRecord); err != nil {
			logrus.WithError(err).Error("crawler.db.CreatePageRecord failed")
			return nil, err
		}
	}

	return pageRecord, nil
}

func (crawler *crawler) scrapingPage(pageUrl string, pageIndex int, buildingId string) error {
	doc, err := crawler.getDocumentFromUrl(pageUrl)
	if err != nil {
		logrus.WithError(err).Error("crawler.getDocumentFromUrl failed")
		return err
	}

	pageRecord, err := crawler.getPageRecord(buildingId, pageIndex)
	if err != nil {
		logrus.WithError(err).Error("crawler.getPageRecord failed")
		return err
	}

	doc.Find("section.c-section[id]").Each(func(i int, s *goquery.Selection) {
		if err := crawler.parseFloorSelection(s, buildingId, pageRecord.Pid); err != nil {
			logrus.WithError(err).Error("crawler.parseFloorSelection failed")
			return
		}
	})

	return nil
}

func (crawler *crawler) getBuildingRecord(targetInfo *TargetInfo, lastPageIndex int, title string) (string, error) {
	var err error
	var buildingRecord *db.BuildingRecord

	buildingRecord, err = crawler.db.GetBuildingRecord(targetInfo.Bsn, targetInfo.Sna)
	if err != nil {
		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("crawler.db.GetBuildingRecord failed")
			return "", err
		}

		buildingRecord = &db.BuildingRecord{
			Id:            uuid.New().String(),
			Bsn:           targetInfo.Bsn,
			Sna:           targetInfo.Sna,
			BuildingTitle: title,
			LastPageIndex: lastPageIndex,
		}
		if err = crawler.db.CreateBuildingRecord(buildingRecord); err != nil {
			logrus.WithError(err).Error("crawler.db.CreateBuildingRecord failed")
			return "", err
		}
		return buildingRecord.Id, nil
	}

	if buildingRecord.BuildingTitle != title || buildingRecord.LastPageIndex != lastPageIndex {
		if err = crawler.db.UpdateBuildingRecord(buildingRecord); err != nil {
			logrus.WithError(err).Error("crawler.db.UpdateBuildingRecord failed")
			return "", err
		}
	}
	return buildingRecord.Id, nil
}

func (crawler *crawler) ScrapingBuilding(targetInfo *TargetInfo) error {
	if err := targetInfo.validate(); err != nil {
		logrus.WithError(err).Error("targetInfo.validate failed")
		return err
	}

	lastPageIndex, title, err := crawler.GetBuildingPageAndTitle(targetInfo)
	if err != nil {
		logrus.WithError(err).Error("crawler.GetBuildingPageAndTitle failed")
		return err
	}

	id, err := crawler.getBuildingRecord(targetInfo, lastPageIndex, title)
	if err != nil {
		logrus.WithError(err).Error("crawler.getBuildingRecord failed")
		return err
	}

	for pageIndex := 1; pageIndex <= lastPageIndex; pageIndex++ {
		pageUrl := targetInfo.GetPageUrl(pageIndex)
		if err := crawler.scrapingPage(pageUrl, pageIndex, id); err != nil {
			logrus.WithError(err).Error("crawler.scrapingPage failed")
			return err
		}
		time.Sleep(scrapingInterval)
	}
	return nil
}

func (crawler *crawler) ScrapingBuildingWithUrl(url string) error {
	targetInfo, err := GetTargetInfoFromUrl(url)
	if err != nil {
		logrus.WithError(err).Error("GetTargetInfoFromUrl failed")
		return err
	}
	return crawler.ScrapingBuilding(targetInfo)
}

func getAlternativeCaptcha(response *resty.Response) string {
	re := regexp.MustCompile(`<input type="hidden" name="alternativeCaptcha" value="(\w+)"`)
	match := re.FindStringSubmatch(response.String())
	if len(match) < 2 {
		logrus.Errorf("alternativeCaptcha value not found")
		return ""
	}
	return match[1]
}

func (crawler *crawler) LoginAndKeepCookies(account, password string) error {
	if crawler.client == nil {
		logrus.Error("client is nil, please use NewCrawler to create a new crawler instance")
		return fmt.Errorf("client is nil")
	}

	// Set User-agent and Cookie to pass the login check
	crawler.client.SetHeader("User-agent", "Mozilla/5.0")
	crawler.client.SetCookie(&http.Cookie{Name: "_ga", Value: "c8763"})

	res, err := crawler.client.R().Get(LoginURLPhase1)
	if err != nil {
		logrus.WithError(err).Errorf("GET %s failed", LoginURLPhase1)
		return err
	}
	defer res.RawResponse.Body.Close()

	alternativeCaptcha := getAlternativeCaptcha(res)
	if alternativeCaptcha == "" {
		logrus.Errorf("alternativeCaptcha value not found")
		return fmt.Errorf("alternativeCaptcha value not found")
	}
	logrus.Infof("get alternativeCaptcha success")

	loginData := map[string]string{
		"userid":             account,
		"password":           password,
		"alternativeCaptcha": alternativeCaptcha,
	}
	if _, err = crawler.client.R().SetFormData(loginData).Post(LoginURLPhase2); err != nil {
		logrus.WithError(err).Errorf("POST %s failed", LoginURLPhase2)
		return err
	}

	logrus.Infof("Login success")
	crawler.isSessionActive = true
	return nil
}
