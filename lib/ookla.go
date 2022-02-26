package lib

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"strings"
)

type OoklaPeerStruct struct {
	ID          string `json:"id"`
	Host        string `json:"host"`
	CountryCode string `json:"cc"`
}

func DNSResolve(Domain string) string {
	respDNS, err := http.Get("http://223.5.5.5/resolve?name=" + Domain + "&short=1")
	if err != nil {
		return ""
	}
	buf := make([]byte, 1024)
	DataLength, _ := respDNS.Body.Read(buf)
	var Data []string
	_ = json.Unmarshal(buf[:DataLength], &Data)
	return Data[0]
}

func GetList(GeoIP string) ([]OoklaPeerStruct, error) {
	URL := `/api/js/servers?engine=js&limit=15&https_functional=true` + func() string {
		if GeoIP != "" {
			return "&ip=" + GeoIP
		} else {
			return ""
		}
	}()
	IP := DNSResolve("www.speedtest.net")
	req, err := http.NewRequest("GET", "https://"+"www.speedtest.net"+URL, nil)
	if err != nil {
		return nil, err
	}
	req.RemoteAddr = IP
	client := http.Client{}
	resp, err := client.Do(req)
	bufData := make([]byte, 8192)
	Len, _ := resp.Body.Read(bufData)
	ListRaw := bufData[:Len]
	var PeerList []OoklaPeerStruct
	err = json.Unmarshal(ListRaw, &PeerList)
	if err != nil {
		return nil, err
	}
	return PeerList, nil
}

func GetIP(PeerList *[]OoklaPeerStruct) []net.IP {
	IPList := make([]net.IP, 0)
	IPChannel := make(chan net.IP, 40)
	WebSocketGetIP := func(Host string) {
		dialer := websocket.Dialer{}
		Conn, _, err := dialer.Dial("ws://"+Host+"/ws", nil)
		if err != nil {
			IPChannel <- nil
			return
		}
		defer func(Conn *websocket.Conn) {
			_ = Conn.Close()
		}(Conn)
		err = Conn.WriteMessage(websocket.TextMessage, []byte("GETIP"))
		if err != nil {
			IPChannel <- nil
			return
		}
		_, messageDataByte, err := Conn.ReadMessage()
		if err != nil {
			IPChannel <- nil
			return
		}
		messageData := string(messageDataByte)
		IPString := strings.ReplaceAll(strings.Split(messageData, " ")[1], "\n", "")
		IP := net.ParseIP(IPString)
		IPChannel <- IP
	}
	for _, v := range *PeerList {
		if v.CountryCode != "CN" {
			continue
		}
		go WebSocketGetIP(v.Host)
	}
	Finish := make(chan bool)
	go func() {
		for {
			if len(IPList) == len(*PeerList) {
				Finish <- true
				break
			}
			select {
			case IP := <-IPChannel:
				if IP != nil {
					IPList = append(IPList, IP)
				}
			}
		}
	}()
	_ = <-Finish
	return func() []net.IP {
		TempMap := make(map[string]int)
		TempMap2 := make([]net.IP, 0)
		for _, v := range IPList {
			if v == nil {
				continue
			}
			TempMap[v.String()]++
		}
		for k := range TempMap {
			TempMap2 = append(TempMap2, net.ParseIP(k))
		}
		return TempMap2
	}()
}
