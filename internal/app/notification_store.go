package app

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
)

type notificationChannel struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Enabled       bool              `json:"enabled"`
	Events        []string          `json:"events"`
	Templates     map[string]string `json:"templates"`
	HasWebhook    bool              `json:"hasWebhook"`
	HasSignSecret bool              `json:"hasSignSecret"`
}

type notificationChannelConfig struct {
	Channel    notificationChannel `json:"channel"`
	Webhook    string              `json:"webhook"`
	SignSecret string              `json:"signSecret"`
}

func (s *Store) saveNotificationChannel(c notificationChannelConfig) error {
	raw, e := json.Marshal(c)
	if e != nil {
		return e
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	// 渠道地址本身也是凭证，与签名密钥、模板一起加密，且绑定渠道 ID。
	data := s.aead.Seal(nonce, nonce, raw, []byte("notification-channel:"+c.Channel.ID))
	_, e = s.db.Exec("INSERT INTO notification_channels(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", c.Channel.ID, data)
	return e
}

func (s *Store) decodeNotificationChannel(id string, data []byte) (notificationChannelConfig, error) {
	var c notificationChannelConfig
	n := s.aead.NonceSize()
	if len(data) < n {
		return c, fmt.Errorf("通知渠道数据损坏")
	}
	raw, e := s.aead.Open(nil, data[:n], data[n:], []byte("notification-channel:"+id))
	if e != nil {
		return c, fmt.Errorf("通知渠道无法解密")
	}
	if e = json.Unmarshal(raw, &c); e != nil {
		return c, e
	}
	c.Channel.HasWebhook = c.Webhook != ""
	c.Channel.HasSignSecret = c.SignSecret != ""
	return c, nil
}

func (s *Store) notificationChannel(id string) (notificationChannelConfig, error) {
	var data []byte
	if e := s.db.QueryRow("SELECT data FROM notification_channels WHERE id=?", id).Scan(&data); e != nil {
		return notificationChannelConfig{}, e
	}
	return s.decodeNotificationChannel(id, data)
}

func (s *Store) notificationChannels() ([]notificationChannelConfig, error) {
	rows, e := s.db.Query("SELECT id,data FROM notification_channels ORDER BY rowid")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []notificationChannelConfig{}
	for rows.Next() {
		var id string
		var data []byte
		if e := rows.Scan(&id, &data); e != nil {
			return nil, e
		}
		c, e := s.decodeNotificationChannel(id, data)
		if e != nil {
			return nil, e
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
