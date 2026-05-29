package httpgateway

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"navmesh-go/domains"

	"github.com/wfu-work/nav-common-go-lib/global"
)

func findMapping(host string) (domains.PortMapping, domains.Device, error) {
	var mapping domains.PortMapping
	if err := global.NAV_DB.Where("public_host = ? AND status = ?", normalizeHost(host), int(domains.StatusEnabled)).First(&mapping).Error; err != nil {
		return mapping, domains.Device{}, err
	}
	if mapping.Protocol != "" && mapping.Protocol != "http" && mapping.Protocol != "https" {
		return mapping, domains.Device{}, errors.New("unsupported mapping protocol")
	}
	var device domains.Device
	if err := global.NAV_DB.Where("guid = ? AND status != ?", mapping.DeviceGuid, domains.DeviceStatusDisabled).First(&device).Error; err != nil {
		return mapping, device, err
	}
	return mapping, device, nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return strings.Trim(host, "[]")
}

func sourceIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func requestSourceIP(r *http.Request) string {
	remote := sourceIP(r.RemoteAddr)
	if !trustedForwardedSource(remote) {
		return remote
	}
	for _, header := range []string{"CF-Connecting-IP", "True-Client-IP", "X-Real-IP"} {
		if ip := firstHeaderIP(r.Header.Get(header)); ip != "" {
			return ip
		}
	}
	if ip := firstHeaderIP(r.Header.Get("X-Forwarded-For")); ip != "" {
		return ip
	}
	if ip := firstForwardedForIP(r.Header.Get("Forwarded")); ip != "" {
		return ip
	}
	return remote
}

func trustedForwardedSource(remote string) bool {
	ip := net.ParseIP(strings.TrimSpace(remote))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func firstHeaderIP(value string) string {
	for _, item := range strings.Split(value, ",") {
		if ip := normalizeHeaderIP(item); ip != "" {
			return ip
		}
	}
	return ""
}

func firstForwardedForIP(value string) string {
	for _, group := range strings.Split(value, ",") {
		for _, part := range strings.Split(group, ";") {
			key, raw, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "for") {
				continue
			}
			if ip := normalizeHeaderIP(raw); ip != "" {
				return ip
			}
		}
	}
	return ""
}

func normalizeHeaderIP(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if value == "" || strings.EqualFold(value, "unknown") {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func intToString(value int) string {
	return strconv.Itoa(value)
}
