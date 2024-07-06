package rule

import (
	"fmt"
	"time"

	"github.com/davidleitw/baha/internal/db"
	"github.com/sirupsen/logrus"
)

const (
	DefaultAsylumBsn  = 60076 // 場外
	DefaultInterval   = 30 * time.Second
	DefaultMaxFailure = 20
)

type RuleOption func(*TrackingRule)

func SyncLocalDb(sync bool) RuleOption {
	return func(o *TrackingRule) {
		o.SyncLocalDb = sync
	}
}

func PokeInterval(interval time.Duration) RuleOption {
	return func(o *TrackingRule) {
		o.PokeInterval = interval
	}
}

func DefaultBsn() RuleOption {
	return func(o *TrackingRule) {
		o.Bsn = DefaultAsylumBsn
	}
}

func Bsn(bsn int) RuleOption {
	return func(o *TrackingRule) {
		o.Bsn = bsn
	}
}

func Sna(sna int) RuleOption {
	return func(o *TrackingRule) {
		o.Sna = sna
	}
}

func Id(id string) RuleOption {
	return func(o *TrackingRule) {
		o.AimId = id
	}
}

func NewPostCallback(callback func(*db.FloorRecord)) RuleOption {
	return func(o *TrackingRule) {
		o.NewPostCallback = callback
	}
}

func DefaultNewPostCallback() RuleOption {
	return func(o *TrackingRule) {
		o.NewPostCallback = func(floor *db.FloorRecord) {
			logrus.Infof("New Post: %s", floor.Content)
		}
	}
}

func UpdateLastCallback(callback func(*db.FloorRecord)) RuleOption {
	return func(o *TrackingRule) {
		o.UpdateLastCallback = callback
	}
}

func DefaultUpdateLastCallback() RuleOption {
	return func(o *TrackingRule) {
		o.UpdateLastCallback = func(floor *db.FloorRecord) {
			logrus.Infof("Update Last: %s", floor.Content)
		}
	}
}

func MaxFailure(failure int) RuleOption {
	return func(o *TrackingRule) {
		o.MaxFailure = failure
	}
}

type TrackingRule struct {
	Bsn         int
	Sna         int
	AimId       string
	Url         string
	LastPageUrl string

	LastFloorIndex  int
	LastFloorRecord *db.FloorRecord

	SyncLocalDb  bool
	PokeInterval time.Duration
	MaxFailure   int

	NewPostCallback    func(*db.FloorRecord)
	UpdateLastCallback func(*db.FloorRecord)
}

func NewTrackingRule(opts ...RuleOption) *TrackingRule {
	rule := &TrackingRule{}
	for _, opt := range opts {
		opt(rule)
	}

	if rule.NewPostCallback == nil {
		DefaultNewPostCallback()(rule)
	}

	if rule.UpdateLastCallback == nil {
		DefaultUpdateLastCallback()(rule)
	}

	if rule.Bsn == 0 || rule.Sna == 0 {
		logrus.Errorf("Bsn or Sna is not set")
		return nil
	}

	if rule.AimId == "" {
		logrus.Errorf("AimId is not set")
		return nil
	}

	rule.Url = fmt.Sprintf("https://forum.gamer.com.tw/C.php?bsn=%d&snA=%d&s_author=%s", rule.Bsn, rule.Sna, rule.AimId)
	rule.LastPageUrl = rule.Url + "&last=1#down"

	return rule
}

func (rule *TrackingRule) GetInterval() time.Duration {
	if rule.PokeInterval == 0 {
		return DefaultInterval
	}
	return rule.PokeInterval
}

func (rule *TrackingRule) GetMaxFailure() int {
	if rule.MaxFailure == 0 {
		return DefaultMaxFailure
	}
	return rule.MaxFailure
}
