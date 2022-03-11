package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	AppName    = "OoklaGetIP"
	AppAuthor  = "Yaott"
	AppVersion = "v1.1.3"
)

type OoklaPeer struct {
	Url       string
	Host      string
	IPAndPort string
}

var (
	OoklaCacheGroup struct {
		List []OoklaPeer
		Mu   sync.Mutex
	}
	OoklaCacheTag struct {
		Tag bool
		Mu  sync.Mutex
	}
	IPCache []net.IP
)

var (
	PeerNum   = 15
	LocalIP   = "1.2.4.8"
	CacheMode = true
)

func main() {
	var Params = struct {
		Version bool
		Help    bool
		Mu      uint
		Port    uint64
		LocalIP string
		NoCache bool
	}{}
	flag.BoolVar(&Params.Version, "v", false, "Show Version")
	flag.BoolVar(&Params.Help, "h", false, "Show Help")
	flag.UintVar(&Params.Mu, "mu", 1, "Multiple Exports")
	flag.Uint64Var(&Params.Port, "p", 9012, "Http Server Listen Port")
	flag.StringVar(&Params.LocalIP, "ip", "1.2.4.8", "Local IP")
	flag.BoolVar(&Params.NoCache, "nc", false, "No Cache Peer List")
	flag.Parse()
	CacheMode = !Params.NoCache
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
	if CacheMode {
		fmt.Println(`Cache Mode`)
	}
	fmt.Println(`Starting Server... Listen Port ` + strconv.FormatUint(Params.Port, 10))
	if CacheMode {
		go CacheModule()
	}
	log.Fatalln(SampleHTTPServer("::", strconv.FormatUint(Params.Port, 10), "/"))
}

func CacheModule() {
	ticker := time.NewTicker(20 * time.Minute)
	for {
		<-ticker.C
		OoklaCacheTag.Mu.Lock()
		OoklaCacheTag.Tag = true
		OoklaCacheTag.Mu.Unlock()
	}
}

func SampleHTTPServer(ListenAddr, ListenPort, Path string) error {
	http.HandleFunc(Path, func(w http.ResponseWriter, r *http.Request) {
		Data, err := OoklaGetIPFull(uint(PeerNum), LocalIP)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		DataStr := SliceTranIPToStr(&Data)
		sort.Strings(DataStr)
		DataJson, err := json.Marshal(DataStr)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write(DataJson)
	})
	return http.ListenAndServe(net.JoinHostPort(ListenAddr, ListenPort), nil)
}

func OoklaGetIPFull(Num uint, LocalIP string) ([]net.IP, error) {
	var (
		PeerList []OoklaPeer
		err      error
	)
	if CacheMode {
		OoklaCacheGroup.Mu.Lock()
		defer OoklaCacheGroup.Mu.Unlock()
		if len(OoklaCacheGroup.List) > 0 {
			CacheFlushTag := false
			OoklaCacheTag.Mu.Lock()
			if OoklaCacheTag.Tag {
				CacheFlushTag = true
			}
			OoklaCacheTag.Mu.Unlock()
			if CacheFlushTag {
				PeerList, err = OoklaGetAllPeer(Num, LocalIP)
				if err == nil {
					OoklaCacheGroup.List = PeerList
				} else {
					if len(IPCache) <= 0 {
						return nil, err
					} else {
						return IPCache, nil
					}
				}
			} else {
				PeerList = OoklaCacheGroup.List
			}
		} else {
			PeerList, err = OoklaGetAllPeer(Num, LocalIP)
			if err != nil {
				if len(IPCache) <= 0 {
					return nil, err
				} else {
					return IPCache, nil
				}
			}
			OoklaCacheGroup.List = PeerList
		}
	} else {
		PeerList, err = OoklaGetAllPeer(Num, LocalIP)
		if err != nil {
			if len(IPCache) <= 0 {
				return nil, err
			} else {
				return IPCache, nil
			}
		}
	}
	IPList, err := OoklaGetAllIP(&PeerList)
	if err != nil {
		if len(IPCache) <= 0 {
			return nil, err
		} else {
			return IPCache, nil
		}
	} else {
		return IPList, nil
	}
}

func HTTPDNSResolveFunc(Host string) (net.IP, error) {
	DNSIP := `223.5.5.5`
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
	client := http.Client{
		Timeout: 2 * time.Second,
	}
	req, _ := http.NewRequest(http.MethodGet, URLGen("https", DNSIP, "/resolve", QueryMap), nil)
	var (
		respDNS *http.Response
		Num     int = 0
	)
	for {
		respDNS, err = client.Do(req)
		if err != nil {
			Num++
		} else {
			break
		}
		if Num == 3 {
			break
		}
	}
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

func OoklaGetAllPeer(Num uint, LocalIP string) ([]OoklaPeer, error) {
	PeerGetURL := `https://www.speedtest.net/api/js/servers`
	PeerGetURLQuery := make(map[string]string)
	PeerGetURLQuery["engine"] = "js"
	PeerGetURLQuery["https_functional"] = "true"
	if LocalIP != "" {
		PeerGetURLQuery["ip"] = LocalIP
	}
	if Num == 0 {
		PeerGetURLQuery["limit"] = "1"
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
	IP, err := HTTPDNSResolveFunc(PeerGetURLParse.Host)
	if err != nil {
		return nil, err
	}
	req.RemoteAddr = net.JoinHostPort(IP.String(), "443")
	var resp *http.Response
	client := http.Client{
		Timeout: 3 * time.Second,
	}
	var RetryNum int = 0
	for {
		resp, err = client.Do(req)
		if err != nil {
			RetryNum++
		} else {
			break
		}
		if RetryNum == 3 {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	bufData := make([]byte, 20480)
	Len, _ := resp.Body.Read(bufData)
	ListRaw := bufData[:Len]
	type PeerListRawStruct struct {
		Host        string `json:"host"`
		CountryCode string `json:"cc"`
	}
	var PeerListRaw []PeerListRawStruct
	err = json.Unmarshal(ListRaw, &PeerListRaw)
	if err != nil {
		return nil, err
	}
	PeerList := make([]OoklaPeer, 0)
	PeerListChan := make(chan OoklaPeer, len(PeerListRaw))
	ResolvePeerIP := func(PeerInfo PeerListRawStruct) (OoklaPeer, error) {
		if PeerInfo.CountryCode != "CN" {
			return OoklaPeer{}, errors.New("not in china")
		} else {
			Host, Port, err := net.SplitHostPort(PeerInfo.Host)
			if err != nil {
				return OoklaPeer{}, err
			}
			ResolveIP, err := HTTPDNSResolveFunc(Host)
			if err != nil {
				return OoklaPeer{}, err
			}
			u := &url.URL{
				Scheme: "wss",
				Host:   PeerInfo.Host,
				Path:   "/ws",
			}
			return OoklaPeer{
				Url:       u.String(),
				Host:      Host,
				IPAndPort: net.JoinHostPort(ResolveIP.String(), Port),
			}, nil
		}
	}
	var wg sync.WaitGroup
	for _, v := range PeerListRaw {
		wg.Add(1)
		go func(Value PeerListRawStruct) {
			defer wg.Done()
			TempInfo, err := ResolvePeerIP(Value)
			if err != nil {
				return
			}
			PeerListChan <- TempInfo
		}(v)
	}
	wg.Wait()
	for {
		BreakTag := false
		select {
		case Data := <-PeerListChan:
			PeerList = append(PeerList, Data)
		default:
			BreakTag = true
		}
		if BreakTag {
			break
		}
	}
	if len(PeerList) <= 0 {
		return nil, errors.New("peer list is nil")
	}
	return PeerList, nil
}

func OoklaGetAllIP(PeerList *[]OoklaPeer) ([]net.IP, error) {
	if len(*PeerList) <= 0 {
		return nil, errors.New("PeerList is nil")
	}
	var wg sync.WaitGroup
	IPGetChannel := make(chan net.IP, len(*PeerList))
	GetIPDO := func(PeerInfo OoklaPeer) {
		defer func() {
			wg.Done()
		}()
		u := url.URL{
			Scheme: "wss",
			Host:   PeerInfo.Host,
			Path:   "/ws",
		}
		WebSocketDialer := websocket.Dialer{
			NetDial: func(network, addr string) (net.Conn, error) {
				Conn, err := net.Dial(network, PeerInfo.IPAndPort)
				if err != nil {
					return nil, err
				}
				return Conn, nil
			},
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 3 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
				ServerName:         PeerInfo.Host,
			},
		}
		HTTPRequest := func() http.Header {
			RequestHttpHeader := make(map[string][]string)
			RequestHttpHeader["Accept-Encoding"] = []string{"gzip,deflate,br"}
			RequestHttpHeader["Accept-Language"] = []string{"zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6"}
			RequestHttpHeader["Cache-Control"] = []string{"no-cache"}
			RequestHttpHeader["Origin"] = []string{"https://www.speedtest.net"}
			RequestHttpHeader["Host"] = []string{PeerInfo.Host}
			RequestHttpHeader["Pragma"] = []string{"no-cache"}
			RequestHttpHeader["User-Agent"] = []string{`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.102 Safari/537.36 Edg/98.0.1108.62`}
			RequestHttpHeader["Dnt"] = []string{"1"}
			if len(RequestHttpHeader) == 0 {
				return nil
			} else {
				return RequestHttpHeader
			}
		}()
		Conn, _, err := WebSocketDialer.Dial(u.String(), HTTPRequest)
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
	for _, v := range *PeerList {
		wg.Add(1)
		go GetIPDO(v)
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
	if len(IPSlice) <= 0 {
		if len(IPCache) > 0 {
			return IPCache, nil
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
	IPCache = IPSliceReal
	return IPSliceReal, nil
}

func SliceTranIPToStr(IPSlice *[]net.IP) []string {
	StrSlice := make([]string, 0)
	for _, v := range *IPSlice {
		StrSlice = append(StrSlice, v.String())
	}
	return StrSlice
}
