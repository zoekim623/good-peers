package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Peers []Peer

func (p Peers) Len() int {
	return len(p)
}

func (p Peers) Less(i, j int) bool {
	return p[i].Speed < p[j].Speed
}

func (p Peers) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type Peer struct {
	NodeId string `json:"node_id"`
	Url    string `json:"url"`
	Speed  time.Duration
}

type NetInfo struct {
	NPeers string `json:"n_peers"`
	Peers  []Peer `json:"peers"`
}

func main() {
	rpc := flag.String("rpc", "https://sei-rpc.polkachu.com", "rpc url")
	n := flag.Int("n", 30, "number of peers")
	ms := flag.Int("ms", 1000, "maximum time(ms)")

	flag.Parse()
	ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
	fmt.Println("rpc: ", *rpc)
	peers, err := getPeers(ctx, *rpc)
	if err != nil {
		panic("failed to fetch peers info")
	}

	var goodPeers Peers
	for _, peer := range peers {
		speed, err := checkPeerSpeed(peer.Url)
		if err != nil {
			fmt.Println("failed to check speed - ", strings.TrimPrefix(peer.Url, "mconn://"))
			continue
		}

		if speed > time.Duration(*ms)*time.Millisecond {
			fmt.Println("too late peers - ", strings.TrimPrefix(peer.Url, "mconn://"))
			continue
		}

		peer.Speed = speed
		goodPeers = append(goodPeers, peer)
	}
	sort.Sort(goodPeers)

	var selectedPeers []string
	for idx, peer := range goodPeers {
		if idx >= *n {
			break
		}
		url := strings.TrimPrefix(peer.Url, "mconn://")
		fmt.Println("#", idx+1, " ", url, " speed: ", peer.Speed)
		selectedPeers = append(selectedPeers, url)
	}

	fmt.Println("total peers: ", len(selectedPeers))
	persistentPeers := strings.Join(selectedPeers, ",")
	fmt.Println(persistentPeers)
}

func checkPeerSpeed(url string) (time.Duration, error) {
	ipPort := strings.Split(url, "@")
	if len(ipPort) != 2 {
		return time.Hour, errors.New("invalid url") // timeout == 1 hour
	}

	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", ipPort[1], 3*time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	speed := time.Since(startTime)
	fmt.Printf("url: %s, speed: %d(ms)\n", ipPort[1], speed/time.Millisecond)
	return speed, nil
}

func getPeers(ctx context.Context, url string) ([]Peer, error) {
	netInfoUrl := fmt.Sprintf("%s/net_info", url)
	resp, err := http.Get(netInfoUrl)
	if err != nil {
		select {
		case <-ctx.Done():
			fmt.Println("request canceled or time out", ctx.Err())
			return nil, ctx.Err()
		default:
			fmt.Println("failed to fetch netinfo", err)
			return nil, err
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("failed to read response body", err)
		return nil, err
	}

	var netInfo NetInfo
	err = json.Unmarshal(body, &netInfo)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return nil, err
	}
	return netInfo.Peers, nil
}
