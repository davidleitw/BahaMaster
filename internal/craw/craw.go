package craw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davidleitw/baha/internal/baha"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

const (
	LoginPhase1Url = "https://user.gamer.com.tw/login.php"
	LoginPhase2Url = "https://user.gamer.com.tw/ajax/do_login.php"
	ExtendReplyUrl = "https://forum.gamer.com.tw/ajax/moreCommend.php?"

	randomSleepSecond = 2
)

type Crawler interface {
	getDocumentFromUrl(url string) (*goquery.Document, error)

	GetBuildingPageAndTitle(targetInfo *TargetInfo) (int, string, error)

	scrapingPage(url string) (*baha.PageRecord, error)

	ScrapingBuilding(targetInfo *TargetInfo) (*baha.BuildingRecord, error)

	ScrapingBuildingWithUrl(url string) (*baha.BuildingRecord, error)

	LoadAuthCookies(userInfo *UserInfo) error
}

type crawler struct {
	client          *resty.Client
	isSessionActive bool
}

func NewCrawler() Crawler {
	return &crawler{
		client:          resty.New(),
		isSessionActive: false,
	}
}

var _ Crawler = (*crawler)(nil)

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

func (crawler *crawler) parseRepliesWithExtendAPI(s *goquery.Selection, record *baha.FloorRecord) *baha.FloorRecord {
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

	extendUrl := fmt.Sprintf("%sbsn=%d&snB=%d&returnHtml=0", ExtendReplyUrl, bsn, snb)
	res, err := crawler.client.R().Get(extendUrl)
	if err != nil {
		logrus.WithError(err).Errorf("GET %s failed", extendUrl)
		return record
	}

	extendReplyResponse := map[string]interface{}{}
	if err := json.Unmarshal(res.Body(), &extendReplyResponse); err != nil {
		logrus.WithError(err).Error("Failed to unmarshal JSON")
		return record
	}
	for index, reply := range extendReplyResponse {
		replyIndex, err := strconv.Atoi(index)
		if err != nil {
			continue
		}
		switch r := reply.(type) {
		case map[string]interface{}:
			record.Messages = append(record.Messages, &baha.ReplyRecord{
				ReplyIndex: replyIndex,
				AuthorName: r["nick"].(string),
				AuthorId:   r["userid"].(string),
				Content:    r["comment"].(string),
			})
		default:
			logrus.Errorf("Unexpected type: %T", r)
		}
	}
	return record
}

func (crawler *crawler) parseReplies(s *goquery.Selection, record *baha.FloorRecord) *baha.FloorRecord {
	s.Find("div.c-reply__item>div>div.reply-content").Each(func(i int, s *goquery.Selection) {
		contentUserSection := s.Find("a.reply-content__user")
		authorName := contentUserSection.Text()
		authorId, exist := contentUserSection.Attr("href")
		if !exist {
			logrus.Errorf("contentUserSection.Attr href not found")
			return
		}

		record.Messages = append(record.Messages, &baha.ReplyRecord{
			ReplyIndex: i,
			AuthorName: authorName,
			AuthorId:   getAuthorIdFromHref(authorId),
			Content:    s.Find("article.c-article>span.comment_content").Text(),
		})
	})
	return record
}

func (crawler *crawler) parseFloorSelection(s *goquery.Selection) *baha.FloorRecord {
	record := &baha.FloorRecord{}

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
	needExtendReplyMessage := replyContentSection.Find("div.nocontent").Length() != 0
	if needExtendReplyMessage {
		return crawler.parseRepliesWithExtendAPI(replyContentSection, record)
	}
	return crawler.parseReplies(replyContentSection, record)
}

func (crawler *crawler) scrapingPage(pageUrl string) (*baha.PageRecord, error) {
	doc, err := crawler.getDocumentFromUrl(pageUrl)
	if err != nil {
		logrus.WithError(err).Error("crawler.getDocumentFromUrl failed")
		return nil, err
	}

	pageRecord := &baha.PageRecord{
		FloorRecords: []*baha.FloorRecord{},
	}

	doc.Find("section.c-section[id]").Each(func(i int, s *goquery.Selection) {
		floorRecord := crawler.parseFloorSelection(s)
		if floorRecord == nil {
			return
		}

		if len(floorRecord.Messages) != 0 {
			sort.Slice(floorRecord.Messages, func(i, j int) bool {
				return floorRecord.Messages[i].ReplyIndex < floorRecord.Messages[j].ReplyIndex
			})
		}
		pageRecord.FloorRecords = append(pageRecord.FloorRecords, floorRecord)
	})

	return pageRecord, nil
}

func randomSleep(second int) {
	time.Sleep(time.Duration(second) * time.Second)
}

func (crawler *crawler) ScrapingBuilding(targetInfo *TargetInfo) (*baha.BuildingRecord, error) {
	if err := targetInfo.selfValidate(); err != nil {
		logrus.WithError(err).Error("targetInfo.selfValidate failed")
		return nil, err
	}

	lastPageIndex, title, err := crawler.GetBuildingPageAndTitle(targetInfo)
	if err != nil {
		logrus.WithError(err).Error("crawler.GetBuildingPageAndTitle failed")
		return nil, err
	}

	buildingRecord := &baha.BuildingRecord{
		Bsn:           targetInfo.Bsn,
		Sna:           targetInfo.Sna,
		BuildingTitle: title,
		Pages:         make([]*baha.PageRecord, lastPageIndex),
	}

	for i := 1; i <= lastPageIndex; i++ {
		pageUrl := targetInfo.GetPageUrl(i)
		pageRecord, err := crawler.scrapingPage(pageUrl)
		if err != nil {
			logrus.WithError(err).Error("crawler.scrapingPage failed")
			return nil, err
		}
		pageRecord.Bsn = targetInfo.Bsn
		pageRecord.Sna = targetInfo.Sna
		buildingRecord.Pages[i-1] = pageRecord

		randomSleep(randomSleepSecond)
	}

	if firstPage := buildingRecord.Pages[0]; firstPage != nil && len(firstPage.FloorRecords) > 0 {
		buildingRecord.PosterFloor = firstPage.FloorRecords[0]
	}

	return buildingRecord, nil
}

func (crawler *crawler) ScrapingBuildingWithUrl(url string) (*baha.BuildingRecord, error) {
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

func (crawler *crawler) LoadAuthCookies(userInfo *UserInfo) error {
	if crawler.client == nil {
		logrus.Error("client is nil, please use NewCrawler to create a new crawler instance")
		return fmt.Errorf("client is nil")
	}

	// Set User-agent and Cookie to pass the login check
	crawler.client.SetHeader("User-agent", "Mozilla/5.0")
	crawler.client.SetCookie(&http.Cookie{Name: "_ga", Value: "c8763"})

	res, err := crawler.client.R().Get(LoginPhase1Url)
	if err != nil {
		logrus.WithError(err).Errorf("GET %s failed", LoginPhase1Url)
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
		"userid":             userInfo.Account,
		"password":           userInfo.Password,
		"alternativeCaptcha": alternativeCaptcha,
	}
	if _, err = crawler.client.R().SetFormData(payload).Post(LoginPhase2Url); err != nil {
		logrus.WithError(err).Errorf("POST %s failed", LoginPhase2Url)
		return err
	}

	logrus.Infof("Login success")
	crawler.isSessionActive = true
	return nil
}

func (crawler *crawler) getDocumentFromUrl(url string) (*goquery.Document, error) {
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
