package extensions

import (
	"sync"

	"github.com/imroc/req/v3"

	"github.com/gocolly/colly/v2"
)

type ReqDriver struct {
	LimitRules []*colly.LimitRule
	Client     *req.Client
	lock       *sync.RWMutex
}
