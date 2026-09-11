// Package wedrive 封装企业微信微盘（wedrive）API 调用，用于上传会议纪要等文本文件。
// 兼容私有化部署：API 基地址与接口前缀均可配置（默认前缀 /cgi-bin）。
package wedrive

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Config 微盘调用配置。
type Config struct {
	APIBase     string        // API 基地址，如 https://weixin.moutai.com.cn:8443
	APIPrefix   string        // 接口前缀，默认 /cgi-bin
	CorpID      string        // 企业 ID
	CorpSecret  string        // 自建应用密钥
	SpaceID     string        // 目标共享空间 ID
	FolderID    string        // 目标文件夹 fileid；为空时上传到空间根目录
	TLSInsecure bool          // 跳过证书校验（私有化自签证书）
	Timeout     time.Duration // 请求超时，默认 30s
}

// Client 微盘客户端。
type Client struct {
	cfg     Config
	http    *http.Client
	tokenMu sync.Mutex
	token   string
	expires time.Time
}

// New 构造微盘客户端。Timeout <= 0 时默认 30s。
func New(cfg Config) *Client {
	cfg.APIPrefix = strings.TrimSpace(cfg.APIPrefix)
	if cfg.APIPrefix == "" {
		cfg.APIPrefix = "/cgi-bin"
	}
	if !strings.HasPrefix(cfg.APIPrefix, "/") {
		cfg.APIPrefix = "/" + cfg.APIPrefix
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	tr := &http.Transport{}
	if cfg.TLSInsecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 私有化自签证书场景
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout, Transport: tr},
	}
}

// Enabled 返回配置是否满足调用条件（企业 ID、密钥、空间、基地址齐备）。
func (c *Config) Enabled() bool {
	return c.APIBase != "" && c.CorpID != "" && c.CorpSecret != "" && c.SpaceID != ""
}

// uploadFile 调用微盘 file_upload 接口上传一个文本文件。
// 返回新建文件的 fileid。
func (c *Client) UploadText(fileName string, content []byte) (string, error) {
	token, err := c.accessToken()
	if err != nil {
		return "", fmt.Errorf("获取微盘 access_token 失败: %w", err)
	}
	fatherID := c.cfg.FolderID
	if fatherID == "" {
		fatherID = c.cfg.SpaceID // 空间根目录的 fatherid 即 spaceid
	}
	body, _ := json.Marshal(map[string]string{
		"spaceid":             c.cfg.SpaceID,
		"fatherid":            fatherID,
		"file_name":           fileName,
		"file_base64_content": base64.StdEncoding.EncodeToString(content),
	})
	u := c.cfg.APIBase + c.cfg.APIPrefix + "/wedrive/file_upload?access_token=" + url.QueryEscape(token)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	var resp struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		FileID  string `json:"fileid"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return "", err
	}
	if resp.Errcode != 0 {
		return "", &APIError{Code: resp.Errcode, Msg: resp.Errmsg}
	}
	return resp.FileID, nil
}

// accessToken 获取 access_token，带进程内缓存（官方有效期 7200s，提前 300s 刷新）。
func (c *Client) accessToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" && time.Now().Before(c.expires.Add(-300*time.Second)) {
		return c.token, nil
	}
	q := url.Values{}
	q.Set("corpid", c.cfg.CorpID)
	q.Set("corpsecret", c.cfg.CorpSecret)
	u := c.cfg.APIBase + c.cfg.APIPrefix + "/gettoken?" + q.Encode()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Errcode     int    `json:"errcode"`
		Errmsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := c.doJSON(req, &resp); err != nil {
		return "", err
	}
	if resp.Errcode != 0 {
		return "", &APIError{Code: resp.Errcode, Msg: resp.Errmsg}
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("access_token 为空")
	}
	if resp.ExpiresIn > 0 {
		c.expires = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	} else {
		c.expires = time.Now().Add(7200 * time.Second)
	}
	c.token = resp.AccessToken
	return c.token, nil
}

// doJSON 发送请求并解析 JSON 响应。
func (c *Client) doJSON(req *http.Request, out any) error {
	r, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", r.StatusCode, truncate(string(data), 300))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析响应失败: %w (body=%s)", err, truncate(string(data), 300))
	}
	return nil
}

// truncate 截断到 n 字节，且不会切开多字节字符（中文错误响应直接按字节切会产生乱码）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// APIError 企业微信接口返回的业务错误。
type APIError struct {
	Code int
	Msg  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("errcode %d: %s", e.Code, e.Msg)
}
