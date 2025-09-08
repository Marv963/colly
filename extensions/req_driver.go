package extensions

import (
	"net/http"
	"net/url"
	"sync"
	"time"

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

func (h *reqDriver) GetMatchingRule(domain string) *colly.LimitRule {
	if h.LimitRules == nil {
		return nil
	}
	h.lock.RLock()
	defer h.lock.RUnlock()
	for _, r := range h.LimitRules {
		if r.Match(domain) {
			return r
		}
	}
	return nil
}

func (h *reqDriver) Cache(request *http.Request, bodySize int, checkRequestHeadersFunc colly.CheckRequestHeadersFunc, checkResponseHeadersFunc colly.CheckResponseHeadersFunc, cacheDir string, cacheExpiration time.Duration) (*colly.Response, error) {
	return nil, nil
}

func (h *reqDriver) Do(request *http.Request, bodySize int, checkRequestHeadersFunc colly.CheckRequestHeadersFunc, checkResponseHeadersFunc colly.CheckResponseHeadersFunc) (*colly.Response, error) {
	return nil, nil
}

func (h *reqDriver) Limit(rule *colly.LimitRule) error {
	h.lock.Lock()
	if h.LimitRules == nil {
		h.LimitRules = make([]*colly.LimitRule, 0, 8)
	}
	h.LimitRules = append(h.LimitRules, rule)
	h.lock.Unlock()
	return rule.Init()
}

func (h *reqDriver) Limits(rules []*colly.LimitRule) error {
	for _, r := range rules {
		if err := h.Limit(r); err != nil {
			return err
		}
	}
	return nil
}

func (h *reqDriver) Jar(j http.CookieJar) {
	h.Client.SetCookieJar(j)
}

func (h *reqDriver) GetJar() http.CookieJar {
	return h.Client.GetClient().Jar
}

func (h *reqDriver) Transport(t http.RoundTripper) {
	client := h.Client.GetClient()
	client.Transport = t
}

func (h *reqDriver) Timeout(t time.Duration) {
	h.Client.SetTimeout(t)
}

func (h *reqDriver) GetTimeout() time.Duration {
	return h.Client.GetClient().Timeout
}

func (h *reqDriver) Proxy(pf colly.ProxyFunc) {
	client := h.Client.GetClient()
	if tr, ok := client.Transport.(*http.Transport); ok && tr != nil {
		tr.Proxy = pf
		tr.DisableKeepAlives = true
		client.Transport = tr
	} else {
		client.Transport = &http.Transport{
			Proxy:             pf,
			DisableKeepAlives: true,
		}
	}
}

func (h *reqDriver) SetCookies(url *url.URL, cookies []*http.Cookie) error {
	client := h.Client.GetClient()
	if client.Jar == nil {
		return colly.ErrNoCookieJar
	}

	client.Jar.SetCookies(url, cookies)

	return nil
}

func (h *reqDriver) Cookies(url *url.URL) []*http.Cookie {
	cookies, _ := h.Client.GetCookies(url.String())

	return cookies
}

func (h *reqDriver) CheckRedirect(f func(req *http.Request, via []*http.Request) error) {
	h.Client.GetClient().CheckRedirect = f
}

func (h *reqDriver) SetClient(client *http.Client) {
}
