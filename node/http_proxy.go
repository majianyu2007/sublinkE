package node

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type HTTPProxy struct {
	Name     string
	Server   string
	Port     int
	Username string
	Password string
	TLS      bool
}

func DecodeHTTPProxyURL(rawURL string) (HTTPProxy, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return HTTPProxy{}, fmt.Errorf("parse HTTP proxy URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return HTTPProxy{}, fmt.Errorf("unsupported HTTP proxy scheme %q", u.Scheme)
	}

	host, portText, err := net.SplitHostPort(u.Host)
	if err != nil {
		return HTTPProxy{}, fmt.Errorf("split HTTP proxy address: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return HTTPProxy{}, fmt.Errorf("parse HTTP proxy port: %w", err)
	}

	proxy := HTTPProxy{
		Name:   u.Fragment,
		Server: host,
		Port:   port,
		TLS:    strings.EqualFold(u.Scheme, "https"),
	}
	if proxy.Name == "" {
		proxy.Name = u.Host
	}
	if u.User != nil {
		proxy.Username = u.User.Username()
		proxy.Password, _ = u.User.Password()
	}
	return proxy, nil
}
