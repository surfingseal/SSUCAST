package main

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Local execution remains loopback-only. Network binding is explicitly enabled
// for the Render web container and always keeps application authentication.
func webSettings(env func(string) string) (string, string, error) {
	mode := env("WEB_DEPLOYMENT")
	if mode != "" && mode != "render" {
		return "", "", fmt.Errorf("unsupported web deployment mode")
	}
	origin := env("WEB_PUBLIC_ORIGIN")
	if origin == "" && mode == "render" {
		origin = env("RENDER_EXTERNAL_URL")
	}
	if origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
			return "", "", fmt.Errorf("public origin must be an HTTPS origin")
		}
		origin = strings.TrimSuffix(origin, "/")
	}
	if mode == "render" && origin == "" {
		return "", "", fmt.Errorf("Render public origin required")
	}
	addr := env("WEB_ADDR")
	if addr == "" {
		if mode == "render" {
			port := env("PORT")
			if port == "" {
				port = "10000"
			}
			addr = net.JoinHostPort("0.0.0.0", port)
		} else {
			addr = "127.0.0.1:8080"
		}
	}
	host, p, e := net.SplitHostPort(addr)
	if e != nil {
		return "", "", e
	}
	port, e := strconv.Atoi(p)
	if e != nil || port < 1 || port > 65535 {
		return "", "", fmt.Errorf("invalid web port")
	}
	ip := net.ParseIP(host)
	if ip == nil || mode == "" && !ip.IsLoopback() || mode == "render" && host != "0.0.0.0" {
		return "", "", fmt.Errorf("web binding not allowed in selected mode")
	}
	return addr, origin, nil
}
