package extensions

import (
	"sync"

	"github.com/imroc/req/v3"

	"github.com/gocolly/colly/v2"
)

type reqDriver struct {
	LimitRules []*colly.LimitRule
	Client     *req.Client
	lock       *sync.RWMutex
}

func NewReqBackend() *reqDriver {
	client := req.NewClient()

	return &reqDriver{
		Client: client,
	}
}
