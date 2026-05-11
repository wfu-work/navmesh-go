package services

type DeviceTypeDefault struct {
	Type      string `json:"type"`
	WebPort   int    `json:"webPort"`
	WebDomain string `json:"webDomain"`
	Remark    string `json:"remark"`
}

var deviceTypeDefaults = map[string]DeviceTypeDefault{
	"ssh":       {Type: "ssh", WebPort: 0, WebDomain: "", Remark: "只启用 SSH 反向接入"},
	"radar":     {Type: "radar", WebPort: 8888, WebDomain: "vpn-ipc.navfirst.com", Remark: "雷达类设备"},
	"radar-one": {Type: "radar-one", WebPort: 8887, WebDomain: "vpn-one.navfirst.com", Remark: "单点雷达类设备"},
	"rain":      {Type: "rain", WebPort: 8889, WebDomain: "vpn-qx.navfirst.com", Remark: "气象/雨量类设备"},
	"hipnames":  {Type: "hipnames", WebPort: 8886, WebDomain: "vpn-hipnames.navfirst.com", Remark: "Hipnames 设备"},
	"dic":       {Type: "dic", WebPort: 8885, WebDomain: "vpn-dic.navfirst.com", Remark: "DIC 设备"},
	"ppp":       {Type: "ppp", WebPort: 8884, WebDomain: "vpn-ppp.navfirst.com", Remark: "PPP 设备"},
	"sag":       {Type: "sag", WebPort: 8883, WebDomain: "vpn-sag.navfirst.com", Remark: "SAG 设备"},
	"data":      {Type: "data", WebPort: 3002, WebDomain: "vpn-data.navfirst.com", Remark: "数据服务类设备"},
}

func GetDeviceTypeDefault(deviceType string) (DeviceTypeDefault, bool) {
	item, ok := deviceTypeDefaults[deviceType]
	return item, ok
}

func ListDeviceTypeDefaults() []DeviceTypeDefault {
	items := make([]DeviceTypeDefault, 0, len(deviceTypeDefaults))
	order := []string{"ssh", "radar", "radar-one", "rain", "hipnames", "dic", "ppp", "sag", "data"}
	for _, key := range order {
		if item, ok := deviceTypeDefaults[key]; ok {
			items = append(items, item)
		}
	}
	return items
}
