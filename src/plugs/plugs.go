package plugs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	databox "github.com/me-box/lib-go-databox"
	"github.com/sausheong/hs1xxplug"
)

var store_endpoint = os.Getenv("DATABOX_STORE_ENDPOINT")

//Some timers and comms channels
var getDataChan = time.NewTicker(time.Second * 10).C
var scanForNewPlugsChan = time.NewTicker(time.Second * 600).C
var newPlugFoundChan = make(chan plug)

//default subnet to scan can be set by plugs.SetScanSubNet
var scan_sub_net = "192.168.1"

//A list of known plugs
var plugList = make(map[string]plug)

func PlugHandler() {

	for {
		select {
		case <-getDataChan:
			fmt.Println("Updating plugs!!")
			go updateReadings()
		case <-scanForNewPlugsChan:
			fmt.Println("Scanning for plugs!!")
			go scanForPlugs()
		case p := <-newPlugFoundChan:
			fmt.Println("New Plug Found!!")
			plugList[p.IP] = p
			go registerPlugWithDatabox(p)
		}
	}
}

func updateReadings() {

	resChan := make(chan *Reading)

	for _, p := range plugList {
		go func(pl plug, c chan<- *Reading) {
			data, err := getPlugData(pl.IP)
			if err == nil {
				c <- data
			}
		}(p, resChan)
	}

	for _ = range plugList {
		res := <-resChan
		jsonString, _ := json.Marshal(res.Emeter.GetRealtime)
		databox.StoreJSONWriteTS(store_endpoint+"/"+macToID(res.System.Mac), string(jsonString))

		jsonString, _ = json.Marshal(res.System.RelayState)
		databox.StoreJSONWriteTS(store_endpoint+"/"+"state-"+macToID(res.System.Mac), string(jsonString))
	}

}

func scanForPlugs() {

	for i := 1; i < 255; i++ {

		go func(j int) {
			ip := scan_sub_net + "." + strconv.Itoa(j)
			url := "http://" + string(ip) + "/"

			if isPlugAtURL(url) == true && isPlugInList(ip) == false {
				fmt.Println("Plug found at", ip)
				res, _ := getPlugInfo(ip)
				p := plug{
					ID:    macToID(res.System.GetSysinfo.Mac),
					IP:    ip,
					Mac:   res.System.GetSysinfo.Mac,
					Name:  res.System.GetSysinfo.DevName,
					State: "Online",
				}
				newPlugFoundChan <- p
			}
		}(i)
	}

}

func registerPlugWithDatabox(p plug) {

	metadata := databox.StoreMetadata{
		Description:    "TP-Link Wi-Fi Smart Plug HS100 power usage",
		ContentType:    "application/json",
		Vendor:         "TP-Link",
		DataSourceType: "TP-Power-Usage",
		DataSourceID:   p.ID,
		StoreType:      "store-json",
		IsActuator:     false,
		Unit:           "",
		Location:       "",
	}
	databox.RegisterDatasource(store_endpoint, metadata)

	metadata = databox.StoreMetadata{
		Description:    "TP-Link Wi-Fi Smart Plug HS100 power state",
		ContentType:    "application/json",
		Vendor:         "TP-Link",
		DataSourceType: "PowerState",
		DataSourceID:   "state-" + p.ID,
		StoreType:      "store-json",
		IsActuator:     true,
		Unit:           "",
		Location:       "",
	}
	databox.RegisterDatasource(store_endpoint, metadata)

}

// SetScanSubNet is used to set the subnet to scan for new plugs
func SetScanSubNet(subnet string) {

	//TODO Validation

	scan_sub_net = subnet
}

// ForceScan will force a scan for new plugs
func ForceScan() {
	go scanForPlugs()
}

// GetPlugList returns a list of known plugs
func GetPlugList() map[string]plug {
	return plugList
}

func getPlugInfo(ip string) (*SysInfo, error) {
	p := hs1xxplug.Hs1xxPlug{IPAddress: ip}
	result, err := p.SystemInfo()
	if err != nil {
		return nil, err
	}
	j := new(SysInfo)
	jsonError := json.Unmarshal([]byte(result), j)
	return j, jsonError
}

func getPlugData(ip string) (*Reading, error) {
	p := hs1xxplug.Hs1xxPlug{IPAddress: ip}
	result, err := p.MeterInfo()
	if err != nil {
		return nil, err
	}
	j := new(Reading)
	jsonError := json.Unmarshal([]byte(result), j)
	return j, jsonError
}

func isPlugAtURL(url string) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)
	if err == nil {
		server := resp.Header.Get("Server")
		fmt.Println("Header::Server ", server)
		if server == "TP-LINK SmartPlug" {
			return true
		}
	}
	return false
}

func isPlugInList(ip string) bool {

	_, exists := plugList[ip]

	if exists {
		return true
	}

	return false
}

func macToID(mac string) string {
	return strings.Replace(mac, ":", "", -1)
}
