package extensions

import (
	"compress/gzip"
	"crypto/sha1"
	"encoding/gob"
	"encoding/hex"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
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
	if cacheDir == "" || request.Method != "GET" || request.Header.Get("Cache-Control") == "no-cache" {
		return h.Do(request, bodySize, checkRequestHeadersFunc, checkResponseHeadersFunc)
	}
	sum := sha1.Sum([]byte(request.URL.String()))
	hash := hex.EncodeToString(sum[:])
	dir := path.Join(cacheDir, hash[:2])
	filename := path.Join(dir, hash)

	if fileInfo, err := os.Stat(filename); err == nil && cacheExpiration > 0 {
		if time.Since(fileInfo.ModTime()) > cacheExpiration {
			_ = os.Remove(filename)
		}
	}

	if file, err := os.Open(filename); err == nil {
		resp := new(colly.Response)
		err := gob.NewDecoder(file).Decode(resp)
		file.Close()
		checkResponseHeadersFunc(request, resp.StatusCode, *resp.Headers)
		if resp.StatusCode < 500 {
			return resp, err
		}
	}
	resp, err := h.Do(request, bodySize, checkRequestHeadersFunc, checkResponseHeadersFunc)
	if err != nil || resp.StatusCode >= 500 {
		return resp, err
	}
	if _, err := os.Stat(dir); err != nil {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return resp, err
		}
	}
	file, err := os.Create(filename + "~")
	if err != nil {
		return resp, err
	}
	if err := gob.NewEncoder(file).Encode(resp); err != nil {
		file.Close()
		return resp, err
	}
	file.Close()
	return resp, os.Rename(filename+"~", filename)
}

func (h *reqDriver) Do(request *http.Request, bodySize int, checkRequestHeadersFunc colly.CheckRequestHeadersFunc, checkResponseHeadersFunc colly.CheckResponseHeadersFunc) (*colly.Response, error) {
	r := h.GetMatchingRule(request.URL.Host)
	if r != nil {
		r.WaitChan <- true
		defer func(r *colly.LimitRule) {
			randomDelay := time.Duration(0)
			if r.RandomDelay != 0 {
				randomDelay = time.Duration(rand.Int63n(int64(r.RandomDelay)))
			}
			time.Sleep(r.Delay + randomDelay)
			<-r.WaitChan
		}(r)
	}
	if !checkRequestHeadersFunc(request) {
		return nil, colly.ErrAbortedBeforeRequest
	}
	res, err := h.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	finalRequest := request
	if res.Request != nil {
		finalRequest = res.Request
	}
	if !checkResponseHeadersFunc(finalRequest, res.StatusCode, res.Header) {
		// closing res.Body (see defer above) without reading it aborts
		// the download
		return nil, colly.ErrAbortedAfterHeaders
	}

	var bodyReader io.Reader = res.Body
	if bodySize > 0 {
		bodyReader = io.LimitReader(bodyReader, int64(bodySize))
	}
	contentEncoding := strings.ToLower(res.Header.Get("Content-Encoding"))
	if !res.Uncompressed && (strings.Contains(contentEncoding, "gzip") || (contentEncoding == "" && strings.Contains(strings.ToLower(res.Header.Get("Content-Type")), "gzip")) || strings.HasSuffix(strings.ToLower(finalRequest.URL.Path), ".xml.gz")) {
		bodyReader, err = gzip.NewReader(bodyReader)
		if err != nil {
			return nil, err
		}
		defer bodyReader.(*gzip.Reader).Close()
	}
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}
	return &colly.Response{
		StatusCode: res.StatusCode,
		Body:       body,
		Headers:    &res.Header,
	}, nil
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
