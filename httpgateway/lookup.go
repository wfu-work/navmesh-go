package httpgateway

import (
	"errors"
	"net"
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

func intToString(value int) string {
	return strconv.Itoa(value)
}
