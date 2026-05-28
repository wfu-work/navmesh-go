package services

import (
	"navmesh-go/domains"

	commonDomains "github.com/wfu-work/nav-common-go-lib/domains"
)

var defaultDeviceGroups = []domains.DeviceGroup{
	{BaseDataEntity: baseData("ssh"), Key: "ssh", Name: "SSH 接入", Icon: "terminal", DefaultWebPort: 0, DefaultDomain: "", Sort: 10, Remark: "只启用SSH接入", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("radar"), Key: "radar", Name: "地基雷达", Icon: "radar-chart", DefaultWebPort: 8888, DefaultDomain: "vpn-ipc.navfirst.com", Sort: 20, Remark: "地基雷达类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("one"), Key: "one", Name: "一维雷达", Icon: "dot-chart", DefaultWebPort: 8887, DefaultDomain: "vpn-one.navfirst.com", Sort: 30, Remark: "一维雷达类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("rain"), Key: "rain", Name: "气象雨量", Icon: "cloud", DefaultWebPort: 8889, DefaultDomain: "vpn-qx.navfirst.com", Sort: 40, Remark: "气象雨量类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("hipnames"), Key: "hipnames", Name: "单机版解算", Icon: "global", DefaultWebPort: 8886, DefaultDomain: "vpn-hipnames.navfirst.com", Sort: 50, Remark: "单机版解算类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("dic"), Key: "dic", Name: "视觉位移", Icon: "experiment", DefaultWebPort: 8885, DefaultDomain: "vpn-dic.navfirst.com", Sort: 60, Remark: "视觉位移类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("sag"), Key: "sag", Name: "边缘弧垂", Icon: "control", DefaultWebPort: 8883, DefaultDomain: "vpn-sag.navfirst.com", Sort: 80, Remark: "边缘弧垂类设备", Status: int(domains.StatusEnabled)},
	{BaseDataEntity: baseData("data"), Key: "data", Name: "数据中台", Icon: "database", DefaultWebPort: 3002, DefaultDomain: "vpn-data.navfirst.com", Sort: 90, Remark: "数据中台类设备", Status: int(domains.StatusEnabled)},
}

func baseData(guid string) commonDomains.BaseDataEntity {
	return commonDomains.BaseDataEntity{Guid: guid}
}
