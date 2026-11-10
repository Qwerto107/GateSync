package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

type notificationSender interface {
	Send(context.Context, notificationChannelConfig, notificationEvent) error
}

type feishuSender struct{ client *http.Client }

var feishuHookPath = regexp.MustCompile(`^/open-apis/bot/v2/hook/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validateFeishuWebhook(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "open.feishu.cn" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || !feishuHookPath.MatchString(u.Path) {
		return fmt.Errorf("请输入 https://open.feishu.cn/open-apis/bot/v2/hook/ 开头的完整飞书机器人 Webhook")
	}
	return nil
}

func feishuSignature(secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (s feishuSender) Send(ctx context.Context, c notificationChannelConfig, event notificationEvent) error {
	if e := validateFeishuWebhook(c.Webhook); e != nil {
		return e
	}
	card, e := renderNotificationCard(c.Channel.Templates[event.Kind], event)
	if e != nil {
		return e
	}
	payload := map[string]any{"msg_type": "interactive", "card": card}
	if c.SignSecret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		payload["timestamp"] = timestamp
		payload["sign"] = feishuSignature(c.SignSecret, timestamp)
	}
	raw, e := json.Marshal(payload)
	if e != nil || len(raw) > 20*1024 {
		return fmt.Errorf("飞书消息不能超过 20 KiB")
	}
	client := s.client
	if client == nil {
		client = ipHTTPClient()
		client.Timeout = 10 * time.Second
		// 禁止跳转，避免机器人地址和卡片内容被重定向到其他服务。
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		defer client.CloseIdleConnections()
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.Webhook, bytes.NewReader(raw))
	if e != nil {
		return fmt.Errorf("无法创建飞书通知请求")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res, e := client.Do(req)
	if e != nil {
		return fmt.Errorf("飞书通知请求失败，请检查网络连接")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("飞书通知返回 HTTP %d", res.StatusCode)
	}
	response, e := io.ReadAll(io.LimitReader(res.Body, 8193))
	if e != nil || len(response) > 8192 {
		return fmt.Errorf("飞书通知响应无效")
	}
	var result struct {
		Code       *int `json:"code"`
		StatusCode *int `json:"StatusCode"`
	}
	if json.Unmarshal(response, &result) != nil {
		return fmt.Errorf("飞书通知响应无效")
	}
	code := result.Code
	if code == nil {
		code = result.StatusCode
	}
	if code == nil {
		return fmt.Errorf("飞书通知响应缺少状态码")
	}
	if *code != 0 {
		reason := map[int]string{19021: "签名校验失败，请检查签名密钥和设备时间", 19022: "出口 IP 不在机器人白名单", 19024: "未匹配机器人关键词，请检查卡片内容"}[*code]
		if reason == "" {
			reason = "请检查机器人设置和卡片模板"
		}
		// 不回显服务端原始消息，避免其中包含 Webhook 或其他敏感内容。
		return fmt.Errorf("飞书通知失败（code=%d）：%s", *code, reason)
	}
	return nil
}
