package craw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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

func (targetInfo *TargetInfo) SelfValidate() error {
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

type Crawler interface {
	getDocumentFromUrl(url string) (*goquery.Document, error)

	GetBuildingPageAndTitle(targetInfo *TargetInfo) (int, string, error)

	scrapingPage(url string, buildingId string) (*db.PageRecord, error)

	ScrapingBuilding(targetInfo *TargetInfo) (*db.BuildingRecord, error)

	ScrapingBuildingWithUrl(url string) (*db.BuildingRecord, error)

	LoadAuthCookies(account, password string) error
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

func (crawler *crawler) parseRepliesWithExtendAPI(s *goquery.Selection, record *db.FloorRecord) *db.FloorRecord {
	onclickValue, exist := s.Find("div.nocontent>a,more-reply").Attr("onclick")
	if !exist {
		logrus.Errorf("extendSection.Find a.more-reply id not found")
		return record
	}

	bsn, snb, err := extractExtendAPIParams(onclickValue)
	if err != nil {
		logrus.WithError(err).Errorf("getExtendRequestId failed")
		return record
	}

	extendUrl := fmt.Sprintf("%sbsn=%d&snB=%d&returnHtml=0", ExtendReplyURL, bsn, snb)
	res, err := crawler.client.R().Get(extendUrl)
	if err != nil {
		logrus.WithError(err).Errorf("GET %s failed", extendUrl)
		return record
	}

	replyRes := map[string]interface{}{}
	if err := json.Unmarshal(res.Body(), &replyRes); err != nil {
		logrus.WithError(err).Error("Failed to unmarshal JSON")
		return record
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
			record.Messages = append(record.Messages, &db.ReplyRecord{
				Fid:        record.Fid,
				ReplyIndex: replyIndex,
				AuthorName: r["nick"].(string),
				AuthorId:   r["userid"].(string),
				Content:    r["comment"].(string),
			})
		default:
			logrus.Errorf("Unexpected type: %T", r)
		}
	}
	sort.Slice(record.Messages, func(i, j int) bool {
		return record.Messages[i].ReplyIndex < record.Messages[j].ReplyIndex
	})
	return record
}

func (crawler *crawler) parseReplies(s *goquery.Selection, record *db.FloorRecord) *db.FloorRecord {
	s.Find("div.c-reply__item>div>div.reply-content").Each(func(i int, s *goquery.Selection) {
		contentUserSection := s.Find("a.reply-content__user")
		name := contentUserSection.Text()
		id, exist := contentUserSection.Attr("href")
		if !exist {
			logrus.Errorf("contentUserSection.Attr href not found")
			return
		}

		record.Messages = append(record.Messages, &db.ReplyRecord{
			Fid:        record.Fid,
			ReplyIndex: i,
			AuthorName: name,
			AuthorId:   getAuthorIdFromHref(id),
			Content:    s.Find("article.c-article>span.comment_content").Text(),
		})
	})
	return record
}

func (crawler *crawler) parseFloorSelection(s *goquery.Selection) *db.FloorRecord {
	record := &db.FloorRecord{
		Fid: uuid.New().String(),
	}

	// get floor id, if it contains "disable" then it's not a valid floor
	floorId, exist := s.Attr("id")
	if !exist || strings.Contains(floorId, "disable") {
		return nil
	}

	mainSection := s.Find("div.c-section__main")

	authorSection := mainSection.Find("div.c-post__header__author")
	floorIndex, exist := authorSection.Find("a.floor").Attr("data-floor")
	if !exist {
		logrus.Errorf("authorSection.Find a.floor data-floor not found")
		return nil
	}
	record.FloorIndex, _ = strconv.Atoi(floorIndex)
	record.AuthorName = authorSection.Find("a.username").Text()
	record.AuthorId = authorSection.Find("a.userid").Text()

	content, err := mainSection.Find("div.c-article__content").Html()
	if err != nil {
		logrus.WithError(err).Errorf("mainSection.Find div.c-article__content failed")
		return nil
	}
	record.Content = content

	replyContentSection := mainSection.Find("div.c-reply")
	// If there are more replies, we need to send another request to get the full content
	if replyContentSection.Find("div.nocontent").Length() != 0 {
		return crawler.parseRepliesWithExtendAPI(replyContentSection, record)
	}
	return crawler.parseReplies(replyContentSection, record)
}

func (crawler *crawler) scrapingPage(pageUrl string, buildingId string) (*db.PageRecord, error) {
	doc, err := crawler.getDocumentFromUrl(pageUrl)
	if err != nil {
		logrus.WithError(err).Error("crawler.getDocumentFromUrl failed")
		return nil, err
	}

	pageRecord := &db.PageRecord{
		Bid:          buildingId,
		Pid:          uuid.New().String(),
		FloorRecords: []*db.FloorRecord{},
	}

	doc.Find("section.c-section[id]").Each(func(i int, s *goquery.Selection) {
		floorRecord := crawler.parseFloorSelection(s)
		if floorRecord == nil {
			return
		}
		floorRecord.Bid = buildingId
		floorRecord.Pid = pageRecord.Pid
		pageRecord.FloorRecords = append(pageRecord.FloorRecords, floorRecord)
	})

	return pageRecord, nil
}

func (crawler *crawler) ScrapingBuilding(targetInfo *TargetInfo) (*db.BuildingRecord, error) {
	if err := targetInfo.SelfValidate(); err != nil {
		logrus.WithError(err).Error("targetInfo.selfValidate failed")
		return nil, err
	}

	lastPageIndex, title, err := crawler.GetBuildingPageAndTitle(targetInfo)
	if err != nil {
		logrus.WithError(err).Error("crawler.GetBuildingPageAndTitle failed")
		return nil, err
	}

	buildingRecord := &db.BuildingRecord{
		Id:            uuid.New().String(),
		Bsn:           targetInfo.Bsn,
		Sna:           targetInfo.Sna,
		BuildingTitle: title,
		LastPageIndex: lastPageIndex,
		Pages:         make([]*db.PageRecord, lastPageIndex),
	}

	for i := 1; i <= lastPageIndex; i++ {
		pageUrl := targetInfo.GetPageUrl(i)
		pageRecord, err := crawler.scrapingPage(pageUrl, buildingRecord.Id)
		if err != nil {
			logrus.WithError(err).Error("crawler.scrapingPage failed")
			return nil, err
		}
		buildingRecord.Pages[i-1] = pageRecord

		time.Sleep(scrapingInterval)
	}

	if firstPage := buildingRecord.Pages[0]; firstPage != nil && len(firstPage.FloorRecords) > 0 {
		buildingRecord.PosterFloor = firstPage.FloorRecords[0]
	}

	return buildingRecord, nil
}

func (crawler *crawler) ScrapingBuildingWithUrl(url string) (*db.BuildingRecord, error) {
	targetInfo, err := GetTargetInfoFromUrl(url)
	if err != nil {
		logrus.WithError(err).Error("GetTargetInfoFromUrl failed")
		return nil, err
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

func (crawler *crawler) LoadAuthCookies(account, password string) error {
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

	payload := map[string]string{
		"userid":             account,
		"password":           password,
		"alternativeCaptcha": alternativeCaptcha,
	}
	if _, err = crawler.client.R().SetFormData(payload).Post(LoginURLPhase2); err != nil {
		logrus.WithError(err).Errorf("POST %s failed", LoginURLPhase2)
		return err
	}

	logrus.Infof("Login success")
	crawler.isSessionActive = true
	return nil
}
