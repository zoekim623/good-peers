package main

import (
	"context"
	"encoding/json"
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

/*
	"node_info": {
		"protocol_version": {
		  "p2p": "8",
		  "block": "11",
		  "app": "0"
		},
		"id": "05106550b6e738d8ce50cb857520124bbcce318f",
		"listen_addr": "35.189.236.126:26656",
		"network": "cataclysm-1",
		"version": "0.37.4",
		"channels": "40202122233038606100",
		"moniker": "cataclysm-1-sentries-0",
		"other": {
		  "tx_index": "on",
		  "rpc_address": "tcp://0.0.0.0:26657"
		}
	},
*/

type NodeInfo struct {
	Id         string `json:"id"`
	ListenAddr string `json:"listen_addr"`
	Other      Other  `json:"other"`
}

type Peer struct {
	NodeInfo NodeInfo `json:"node_info"`
	RemoteIp string   `json:"remote_ip"`
	Url      string   `json:"url"`
	Speed    time.Duration
}

type Other struct {
	RpcAddress string `json:"rpc_address"`
}

type Result struct {
	NPeers string `json:"n_peers"`
	Peers  []Peer `json:"peers"`
}

type NetInfo struct {
	Result Result `json:"result"`
}

func main() {
	rpc := flag.String("rpc", "https://nibiru-rpc.polkachu.com", "rpc url")
	n := flag.Int("n", 30, "number of peers")
	ms := flag.Int("ms", 1000, "maximum time(ms)")

	flag.Parse()
	ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
	peers, err := getPeers(ctx, *rpc)
	fmt.Println(len(peers))
	if err != nil {
		panic("failed to fetch peers info")
	}

	var goodPeers Peers
	for _, peer := range peers {
		url := peer.NodeInfo.ListenAddr
		url = strings.TrimPrefix(url, "tcp://")
		urlParts := strings.Split(url, ":")
		if peer.RemoteIp != "0.0.0.0" {
			url = peer.RemoteIp + ":" + urlParts[1]
		}
		speed, err := checkPeerSpeed(url, peer.RemoteIp)
		if err != nil {
			fmt.Println("failed to check speed - ", strings.TrimPrefix(url, "mconn://"))
			continue
		}

		if speed > time.Duration(*ms)*time.Millisecond {
			fmt.Println("too late peers - ", strings.TrimPrefix(url, "mconn://"))
			continue
		}

		peer.Speed = speed
		peer.Url = url
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

func checkPeerSpeed(url, remoteIp string) (time.Duration, error) {

	startTime := time.Now()
	conn, err := net.DialTimeout("tcp", url, 3*time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	speed := time.Since(startTime)
	fmt.Printf("url: %s, speed: %d(ms)\n", url, speed/time.Millisecond)
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

	return netInfo.Result.Peers, nil
}
