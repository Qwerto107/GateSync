package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func healthcheck() error {
	address := os.Getenv("GATESYNC_ADDRESS")
	if address == "" {
		address = "127.0.0.1:13850"
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("健康检查地址无效：%w", err)
	}
	// 健康检查只请求已运行的服务，不读取凭证或初始化数据库。
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/api/health")
	if err != nil {
		return fmt.Errorf("健康检查失败：%w", err)
	}
	defer response.Body.Close()
	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&result); err != nil || response.StatusCode != http.StatusOK || result.Status != "ok" {
		return fmt.Errorf("健康检查未通过：HTTP %d", response.StatusCode)
	}
	return nil
}
