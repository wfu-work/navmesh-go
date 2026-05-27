package services

import (
	"navmesh-go/domains"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
)

var defaultDeviceGroups = []domains.DeviceGroup{
	{BaseDataEntity: baseData("ssh"), Key: "ssh", Name: "SSH 接入", DefaultWebPort: 0, DefaultDomain: "", Sort: 10, Remark: "只启用SSH反向接入", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("radar"), Key: "radar", Name: "雷达设备", DefaultWebPort: 8888, DefaultDomain: "vpn-ipc.navfirst.com", Sort: 20, Remark: "雷达类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("radar-one"), Key: "radar-one", Name: "单点雷达", DefaultWebPort: 8887, DefaultDomain: "vpn-one.navfirst.com", Sort: 30, Remark: "单点雷达类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("rain"), Key: "rain", Name: "气象雨量", DefaultWebPort: 8889, DefaultDomain: "vpn-qx.navfirst.com", Sort: 40, Remark: "气象雨量类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("hipnames"), Key: "hipnames", Name: "Hipnames 设备", DefaultWebPort: 8886, DefaultDomain: "vpn-hipnames.navfirst.com", Sort: 50, Remark: "Hipnames 设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("dic"), Key: "dic", Name: "DIC 设备", DefaultWebPort: 8885, DefaultDomain: "vpn-dic.navfirst.com", Sort: 60, Remark: "DIC 设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("ppp"), Key: "ppp", Name: "PPP 设备", DefaultWebPort: 8884, DefaultDomain: "vpn-ppp.navfirst.com", Sort: 70, Remark: "PPP 设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("sag"), Key: "sag", Name: "SAG 设备", DefaultWebPort: 8883, DefaultDomain: "vpn-sag.navfirst.com", Sort: 80, Remark: "SAG 设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("data"), Key: "data", Name: "数据服务", DefaultWebPort: 3002, DefaultDomain: "vpn-data.navfirst.com", Sort: 90, Remark: "数据服务类设备", Status: int(domains.StatusEnabled)},
}

func baseData(guid string) commonDomains.BaseDataEntity {
	return commonDomains.BaseDataEntity{Guid: guid}
}
