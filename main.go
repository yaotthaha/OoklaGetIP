package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const (
	AppName    = "OoklaGetIP"
	AppAuthor  = "Yaott"
	AppVersion = "v1.0.0"
)

var (
	HTTPDNS = "223.5.5.5"
	PeerNum = 15
	LocalIP = "1.2.4.8"
)

func main() {
	var Params = struct {
		Version bool
		Help    bool
		Mu      uint
		Port    uint64
		LocalIP string
	}{}
	flag.BoolVar(&Params.Version, "v", false, "Show Version")
	flag.BoolVar(&Params.Help, "h", false, "Show Help")
	flag.UintVar(&Params.Mu, "mu", 1, "Multiple Exports")
	flag.Uint64Var(&Params.Port, "p", 9012, "Http Server Listen Port")
	flag.StringVar(&Params.LocalIP, "ip", "1.2.4.8", "Local IP")
	flag.Parse()
	if Params.Help {
		flag.Usage()
		return
	}
	if Params.Version {
		fmt.Println(AppName + ` ` + AppVersion + ` (Build From ` + AppAuthor + `)`)
		return
	}
	if Params.Mu == 0 || Params.Mu > 16 {
		fmt.Println("Multiple Exports set error, just support 1-16")
		return
	} else {
		PeerNum = int(Params.Mu)
	}
	if Params.Port == 0 || Params.Port > 65535 {
		fmt.Println("Http Server Listen Port set error, just support 1-65535")
		return
	}
	IPCheck := net.ParseIP(Params.LocalIP)
	if IPCheck == nil {
		fmt.Println("Local IP set error")
		return
	} else if !IPCheck.IsGlobalUnicast() {
		fmt.Println("Local IP set error")
		return
	} else {
		LocalIP = IPCheck.String()
	}
	fmt.Println(AppName + ` ` + AppVersion + ` (Build From ` + AppAuthor + `)`)
	fmt.Println(`Starting Server... Listen Port ` + strconv.FormatUint(Params.Port, 10))
	log.Fatalln(SampleHTTPServer("::", strconv.FormatUint(Params.Port, 10), "/"))
}

func SampleHTTPServer(ListenAddr, ListenPort, Path string) error {
	http.HandleFunc(Path, func(w http.ResponseWriter, r *http.Request) {
		Data, err := OoklaGetIPFull(uint(PeerNum), LocalIP, HTTPDNS)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		DataJson, err := json.Marshal(Data)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write(DataJson)
	})
	return http.ListenAndServe(net.JoinHostPort(ListenAddr, ListenPort), nil)
}

func OoklaGetIPFull(Num uint, LocalIP, HTTPDNSSupport string) ([]net.IP, error) {
	PeerList, err := OoklaGetAllPeer(Num, LocalIP, HTTPDNSSupport)
	if err != nil {
		return nil, err
	}
	return OoklaGetAllIP(&PeerList, HTTPDNSSupport)
}

type OoklaPeer struct {
	Host        string `json:"host"`
	CountryCode string `json:"cc"`
}

func HTTPDNSResolveFunc(Host, DNSIP string) (net.IP, error) {
	HostReal, _, err := net.SplitHostPort(Host)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port in address") {
			return nil, err
		} else {
			HostReal = Host
		}
	}
	QueryMap := make(map[string]string)
	QueryMap["name"] = HostReal
	QueryMap["short"] = "1"
	respDNS, err := http.Get(URLGen("https", DNSIP, "/resolve", QueryMap))
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 1024)
	DataLength, _ := respDNS.Body.Read(buf)
	var Data []string
	_ = json.Unmarshal(buf[:DataLength], &Data)
	IP := net.ParseIP(Data[0])
	return IP, nil
}

func URLGen(Scheme, Host, Path string, Query map[string]string) string {
	URL := url.URL{
		Scheme: Scheme,
		Host:   Host,
		Path:   Path,
		RawQuery: strings.Join(func(QueryMap map[string]string) []string {
			QuerySlice := make([]string, 0)
			for k, v := range QueryMap {
				QuerySlice = append(QuerySlice, k+"="+v)
			}
			return QuerySlice
		}(Query), "&"),
	}
	return URL.String()
}

func OoklaGetAllPeer(Num uint, LocalIP, HTTPDNSResolve string) ([]OoklaPeer, error) {
	PeerGetURL := `https://www.speedtest.net/api/js/servers`
	PeerGetURLQuery := make(map[string]string)
	PeerGetURLQuery["engine"] = "js"
	PeerGetURLQuery["https_functional"] = "true"
	if LocalIP != "" {
		PeerGetURLQuery["ip"] = LocalIP
	}
	if Num == 0 {
		PeerGetURLQuery["limit"] = "15"
	} else {
		PeerGetURLQuery["limit"] = strconv.Itoa(int(Num))
	}
	PeerGetURLParse, err := url.Parse(PeerGetURL)
	if err != nil {
		return nil, err
	}
	u := url.URL{
		Scheme: PeerGetURLParse.Scheme,
		Host:   PeerGetURLParse.Host,
		Path:   PeerGetURLParse.Path,
		RawQuery: func() string {
			QueryStringSlice := make([]string, 0)
			for k, v := range PeerGetURLQuery {
				QueryStringSlice = append(QueryStringSlice, k+"="+v)
			}
			return strings.Join(QueryStringSlice, "&")
		}(),
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	if HTTPDNSResolve != "" {
		IP, err := HTTPDNSResolveFunc(PeerGetURLParse.Host, HTTPDNS)
		if err != nil {
			return nil, err
		}
		req.RemoteAddr = IP.String()
	}
	client := http.Client{}
	resp, err := client.Do(req)
	bufData := make([]byte, 20480)
	Len, _ := resp.Body.Read(bufData)
	ListRaw := bufData[:Len]
	var PeerList []OoklaPeer
	err = json.Unmarshal(ListRaw, &PeerList)
	if err != nil {
		return nil, err
	}
	return PeerList, nil
}

func OoklaGetAllIP(PeerList *[]OoklaPeer, HTTPDNSResolve string) ([]net.IP, error) {
	if len(*PeerList) <= 0 {
		return nil, errors.New("PeerList is nil")
	}
	var wg sync.WaitGroup
	IPGetChannel := make(chan net.IP, len(*PeerList))
	GetIPDO := func(PeerInfo OoklaPeer, HTTPDNSSupport string) {
		defer func() {
			wg.Done()
		}()
		if PeerInfo.CountryCode == "CN" {
			u := url.URL{
				Scheme: "ws",
				Host: func() string {
					if HTTPDNSSupport != "" {
						RealHost, DialPort, err := net.SplitHostPort(PeerInfo.Host)
						if err != nil {
							return PeerInfo.Host
						}
						ResolveIP, err := HTTPDNSResolveFunc(RealHost, HTTPDNSSupport)
						if err != nil {
							return PeerInfo.Host
						}
						return net.JoinHostPort(ResolveIP.String(), DialPort)
					} else {
						return PeerInfo.Host
					}
				}(),
				Path: "/ws",
			}
			Conn, _, err := websocket.DefaultDialer.Dial(u.String(), func() http.Header {
				if HTTPDNSSupport != "" {
					RealHost, _, err := net.SplitHostPort(u.Host)
					if err != nil {
						return nil
					}
					if TransToIP := net.ParseIP(RealHost); TransToIP == nil {
						return nil
					}
					RequestHttpHeader := make(map[string][]string)
					RequestHttpHeader["Host"] = []string{RealHost}
					return RequestHttpHeader
				} else {
					return nil
				}
			}())
			if err != nil {
				return
			}
			defer func(Conn *websocket.Conn) {
				_ = Conn.Close()
			}(Conn)
			err = Conn.WriteMessage(websocket.TextMessage, []byte("GETIP"))
			if err != nil {
				return
			}
			_, messageDataByte, err := Conn.ReadMessage()
			if err != nil {
				return
			}
			IPString := strings.ReplaceAll(strings.Split(string(messageDataByte), " ")[1], "\n", "")
			IP := net.ParseIP(IPString)
			if IP != nil {
				IPGetChannel <- IP
			}
		}
	}
	for _, v := range *PeerList {
		wg.Add(1)
		go GetIPDO(v, HTTPDNSResolve)
	}
	wg.Wait()
	IPSlice := make([]net.IP, 0)
	for {
		BreakTag := false
		select {
		case IPGet := <-IPGetChannel:
			IPSlice = append(IPSlice, IPGet)
		default:
			BreakTag = true
		}
		if BreakTag {
			break
		}
	}
	IPSliceReal := make([]net.IP, 0)
	func() {
		TempMap := make(map[string]int)
		for _, v := range IPSlice {
			TempMap[v.String()]++
		}
		for k := range TempMap {
			IPSliceReal = append(IPSliceReal, net.ParseIP(k))
		}
	}()
	return IPSliceReal, nil
}
