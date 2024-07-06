package craw

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davidleitw/baha/internal/db"
	"github.com/go-resty/resty/v2"
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

type Crawler interface {
	LoginAndKeepCookies(account, password string) error

	ParsePage(url string) (*db.PageRecord, error)
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

func (crawler *crawler) parseReplyMessageExtended(selection *goquery.Selection) ([]*db.ReplyRecord, error) {
	onclickValue, exist := selection.Find("div.nocontent>a.more-reply").Attr("onclick")
	if !exist {
		logrus.Errorf("extendSelection.Find a.more-reply id not found")
		return nil, nil
	}

	bsn, snb, err := extractExtendAPIParams(onclickValue)
	if err != nil {
		logrus.WithError(err).Errorf("getExtendRequestId failed")
		return nil, err
	}

	extendUrl := fmt.Sprintf("%sbsn=%d&snB=%d&returnHtml=0", ExtendReplyURL, bsn, snb)
	res, err := crawler.client.R().Get(extendUrl)
	if err != nil {
		logrus.WithError(err).Errorf("GET %s failed", extendUrl)
		return nil, err
	}

	replyRes := map[string]interface{}{}
	if err := json.Unmarshal(res.Body(), &replyRes); err != nil {
		logrus.WithError(err).Error("Failed to unmarshal JSON")
		return nil, err
	}

	replies := make([]*db.ReplyRecord, 0)
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
				ReplyIndex: replyIndex,
				AuthorName: r["nick"].(string),
				AuthorId:   r["userid"].(string),
				Content:    r["comment"].(string),
			}
			replies = append(replies, record)
		default:
			logrus.Errorf("Unexpected type: %T", r)
		}
	}
	return replies, nil
}

func (crawler *crawler) parseReplyMessage(selection *goquery.Selection) ([]*db.ReplyRecord, error) {
	if selection.Find("div.nocontent").Length() != 0 {
		return crawler.parseReplyMessageExtended(selection)
	}

	records := make([]*db.ReplyRecord, 0)
	selection.Find("div.c-reply__item>div>div.reply-content").Each(func(i int, s *goquery.Selection) {
		contentUserSelection := s.Find("a.reply-content__user")
		name := contentUserSelection.Text()
		id, exist := contentUserSelection.Attr("href")
		if !exist {
			logrus.Errorf("contentUserSelection.Attr href not found")
			return
		}

		record := &db.ReplyRecord{
			ReplyIndex: i,
			AuthorName: name,
			AuthorId:   getAuthorIdFromHref(id),
			Content:    s.Find("article.c-article>span.comment_content").Text(),
		}
		records = append(records, record)
	})
	return records, nil
}

func (crawler *crawler) parseFloor(selection *goquery.Selection) (*db.FloorRecord, error) {
	record := &db.FloorRecord{
		Replies: make([]*db.ReplyRecord, 0),
	}

	if disableFloor, exist := selection.Attr("id"); !exist || strings.Contains(disableFloor, "disable") {
		return nil, nil
	}

	mainSelection := selection.Find("div.c-section__main")
	authorSelection := mainSelection.Find("div.c-post__header__author")
	floorIndex, exist := authorSelection.Find("a.floor").Attr("data-floor")
	if !exist {
		logrus.Errorf("authorSelection.Find a.floor data-floor not found")
		return nil, errors.New("floorIndex not found")
	}

	index, err := strconv.Atoi(floorIndex)
	if err != nil {
		logrus.WithError(err).Errorf("strconv.Atoi %s failed", floorIndex)
		return nil, err
	}

	record.AuthorName = authorSelection.Find("a.username").Text()
	record.AuthorId = authorSelection.Find("a.userid").Text()
	record.FloorIndex = index

	content, err := mainSelection.Find("div.c-article__content").Html()
	if err != nil {
		logrus.WithError(err).Errorf("mainSelection.Find div.c-article__content failed")
		return nil, errors.New("content not found")
	}
	record.Content = content

	replies, err := crawler.parseReplyMessage(mainSelection.Find("div.c-reply"))
	if err != nil {
		logrus.WithError(err).Error("crawler.parseReplyMessage failed")
		return nil, err
	}
	record.Replies = replies
	return record, nil
}

func (crawler *crawler) ParsePage(url string) (*db.PageRecord, error) {
	record := &db.PageRecord{
		Floors: make([]*db.FloorRecord, 0),
	}

	doc, err := crawler.getDocumentFromUrl(url)
	if err != nil {
		logrus.WithError(err).Error("crawler.getDocumentFromUrl failed")
		return record, err
	}

	doc.Find("section.c-section[id]").Each(func(i int, s *goquery.Selection) {
		floorRecord, err := crawler.parseFloor(s)
		if floorRecord == nil || err != nil {
			return
		}
		record.Floors = append(record.Floors, floorRecord)
	})

	return record, nil
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
