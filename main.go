package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type IPResult struct {
	Hostname      string
	AverageRTT    float64
	TXTRecords    []string
	IPv6Addresses []string
}

func extractRTT(output string) ([]float64, error) {
	rttPattern := regexp.MustCompile(`time=([\d.]+)`)
	matches := rttPattern.FindAllStringSubmatch(output, -1)

	var rttValues []float64
	for _, match := range matches {
		rttStr := match[1]
		rtt, err := strconv.ParseFloat(rttStr, 64)
		if err != nil {
			return nil, err
		}
		rttValues = append(rttValues, rtt)
	}

	return rttValues, nil
}

func calculateAverage(values []float64) float64 {
	total := 0.0
	for _, val := range values {
		total += val
	}
	return total / float64(len(values))
}

func getHostname(ip string) (string, error) {
	names, err := net.LookupAddr(ip)
	if err != nil {
		return "", err
	}
	if len(names) > 0 {
		return names[0], nil
	}
	return "", nil
}

func getTXTRecords(hostname string) ([]string, error) {
	txts, err := net.LookupTXT(hostname)
	if err != nil {
		return nil, err
	}
	return txts, nil
}

func getIPv6Addresses(hostname string) ([]string, error) {
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, err
	}

	var ipv6Addresses []string
	for _, ip := range ips {
		if ip.To4() == nil && ip.To16() != nil {
			ipv6Addresses = append(ipv6Addresses, ip.String())
		}
	}

	return ipv6Addresses, nil
}

// Function to colorize RTT values based on thresholds
func colorRTT(rtt float64) string {
	nbSpace := '\u00A0'
	switch {
	case rtt < 5:
		return color.GreenString("%.2f%cms", rtt, nbSpace)
	case rtt < 100:
		return color.YellowString("%.2f%cms", rtt, nbSpace)
	default:
		return color.RedString("%.2f%cms", rtt, nbSpace)
	}
}

// Custom IP sorting function
type ByIPv4 []string

func (a ByIPv4) Len() int {
	return len(a)
}

func (a ByIPv4) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a ByIPv4) Less(i, j int) bool {
	ip1 := net.ParseIP(a[i]).To4()
	ip2 := net.ParseIP(a[j]).To4()

	for k := range net.IPv4len {
		if ip1[k] != ip2[k] {
			return ip1[k] < ip2[k]
		}
	}

	return false
}

// ...

func pingAndUpdate(ip string, results map[string]IPResult, mu *sync.Mutex, semaphore chan struct{}, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
		<-semaphore
	}()

	cmd := exec.Command("ping", "-c", "3", "-W", "2", ip)
	cmd.Stdout = nil
	cmd.Stderr = nil

	output, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	rttValues, err := extractRTT(string(output))
	if err != nil {
		return
	}

	average := calculateAverage(rttValues)

	hostname, _ := getHostname(ip)
	txtRecords, _ := getTXTRecords(hostname)
	ipv6Addresses, _ := getIPv6Addresses(hostname)

	mu.Lock()
	results[ip] = IPResult{
		Hostname:      hostname,
		AverageRTT:    average,
		TXTRecords:    txtRecords,
		IPv6Addresses: ipv6Addresses,
	}
	mu.Unlock()
}

func main() {
	color.NoColor = false

	var mu sync.Mutex
	var displayTable bool
	var maxParallel int

	cidrPtr := flag.String("cidr", "", "CIDR notation of the network (e.g., 192.168.1.0/24)")
	flag.BoolVar(&displayTable, "table", false, "Display results in a table (ASCII art)")
	flag.IntVar(&maxParallel, "parallel", 255, "Maximum number of parallel pings")

	flag.Parse()

	if *cidrPtr == "" {
		fmt.Println("Please provide a CIDR notation using the -cidr flag.")
		return
	}

	ipsToPing, err := getIPsFromCIDR(*cidrPtr)
	if err != nil {
		fmt.Println("Invalid CIDR format:", err)
		return
	}

	startTime := time.Now()
	semaphore := make(chan struct{}, maxParallel)

	results := make(map[string]IPResult)
	progressBar := pb.StartNew(len(ipsToPing))
	progressBar.SetMaxWidth(80)
	progressBar.Start()

	var wg sync.WaitGroup

	for _, ip := range ipsToPing {
		ipStr := ip.String()
		wg.Add(1)
		go func(ipStr string) {
			defer progressBar.Increment() // Increment progress bar
			semaphore <- struct{}{}       // Acquire a token from the semaphore
			pingAndUpdate(ipStr, results, &mu, semaphore, &wg)
		}(ipStr)
	}

	wg.Wait()
	progressBar.Finish() // Finish progress bar

	// Sorting IPs in the correct order
	sortedIPs := make([]string, 0, len(results))
	for ip := range results {
		sortedIPs = append(sortedIPs, ip)
	}
	sort.Sort(ByIPv4(sortedIPs))

	if displayTable {
		displayTableResults(results, sortedIPs)
	} else {
		displayListResults(results, sortedIPs)
	}

	fmt.Printf("\nTotal IPs: %d\n", len(results))
	totalSeconds := time.Since(startTime).Seconds()
	fmt.Printf("Total Time: %.1fs\n", totalSeconds)
}

func displayTableResults(results map[string]IPResult, sortedIPs []string) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.AppendHeader(table.Row{"IP", "Hostname", "Average RTT", "TXT Records", "IPv6 Addresses"})

	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, Align: text.AlignLeft},
		{Number: 2, Align: text.AlignLeft},
		{Number: 3, Align: text.AlignRight},
		{Number: 4, Align: text.AlignLeft},
		{Number: 5, Align: text.AlignLeft},
	})

	for _, ip := range sortedIPs {
		data := results[ip]
		hostname := color.CyanString(data.Hostname)
		averageRTT := colorRTT(data.AverageRTT)
		txtRecords := strings.Join(data.TXTRecords, "\n")
		ipv6Addresses := strings.Join(data.IPv6Addresses, "\n")

		t.AppendRow(table.Row{
			color.BlueString(ip),
			hostname,
			averageRTT,
			txtRecords,
			ipv6Addresses,
		})
	}
	t.Render()
}

func displayListResults(results map[string]IPResult, sortedIPs []string) {
	for _, ip := range sortedIPs {
		data, _ := results[ip]
		displayIPInfo(ip, data)
	}
}

func displayIPInfo(ip string, data IPResult) {
	fmt.Printf("IP: %s\n", color.BlueString(ip))

	if data.Hostname != "" {
		fmt.Printf("  Hostname: %s\n", color.CyanString(data.Hostname))
	}

	fmt.Printf("  Average RTT: %s ms\n", colorRTT(data.AverageRTT))

	if len(data.TXTRecords) > 0 {
		fmt.Printf("  TXT Records: %s\n", color.WhiteString(strings.Join(data.TXTRecords, ", ")))
	}

	if len(data.IPv6Addresses) > 0 {
		ipv6Addresses := strings.Join(data.IPv6Addresses, "\n  ")
		fmt.Printf("  IPv6 Addresses: %s\n", color.BlueString(ipv6Addresses))
	}
}

func getIPsFromCIDR(cidr string) ([]net.IP, error) {
	ips := []net.IP{}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return ips, err
	}

	ip := ipNet.IP.Mask(ipNet.Mask)
	for ipNet.Contains(ip) {
		ipv4 := make(net.IP, len(ip))
		copy(ipv4, ip)
		ips = append(ips, ipv4.To4())
		incIP(ip)
	}
	return ips, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
