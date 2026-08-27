package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

func addressDefault() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if n, err := strconv.Atoi(port); err == nil && n >= 1024 && n <= 65535 {
			return net.JoinHostPort("127.0.0.1", port)
		}
	}
	return defaultAddress
}

func validateAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("监听地址必须采用 host:port 格式: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("监听主机必须为回环地址，得到 %q", host)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return fmt.Errorf("监听端口必须在 1024 到 65535 之间")
	}
	return nil
}
