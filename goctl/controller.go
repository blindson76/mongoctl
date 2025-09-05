package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	capi "github.com/hashicorp/consul/api"
	napi "github.com/hashicorp/nomad/api"
	"github.com/reactivex/rxgo/v2"
)

var (
	ncli *napi.Client
)

type MongoReplConfig struct {
	Count         int    `json:"count"`
	Primary       string `json:"primary"`
	Members       string `json:"members"`
	ReplSetId     string `json:"replSetId"`
	ReplSetName   string `json:"repLSetName"`
	OpLogFirstSec uint32 `json:"OpLogFirstSec"`
	OpLogFirstInc uint32 `json:"OpLogFirstInc"`
	OpLogLastSec  uint32 `json:"OpLogLastSec"`
	OpLogLasttInc uint32 `json:"OpLogLasttInc"`
}

func controllerJob() {
	var activeMembers []string
	var activeMembersMap MongoStatusMap
	monitorMode := false
	if item, _, err := kv.Get("config/mongo", nil); err == nil && item != nil {
		var startupConfig MongoReplConfig
		log.Println(item)
		if err = json.Unmarshal(item.Value, &startupConfig); err == nil {
			log.Println("read startup config", startupConfig)
			log.Println("Starting in monitor mode")
			monitorMode = true
		}
	}

	config := configurer(context.Background(), monitorMode)
	for cfg := range config.Observe() {
		if monitorMode {
			log.Println("Status changed", cfg)

		} else {

			log.Println("new conf")
			nodes := cfg.V.(MongoStatusMap)
			sorted := sortMembersByOplog(nodes)
			last := int(math.Min(float64(len(sorted)), 3))
			selected := sorted[0:last]
			activeMembersMap = make(MongoStatusMap)

			for _, nodeId := range selected {
				activeMembersMap[nodeId] = nodes[nodeId]
			}
			activeMembers = selected
			log.Println(activeMembers, activeMembersMap)
			memberStr := []string{}
			for _, m := range activeMembers {
				memb := activeMembersMap[m]
				memberStr = append(memberStr, fmt.Sprintf("%s:%s:%s:%s", memb.NodeId, memb.NodeName, memb.MongoAddr, memb.MongoPort))
			}

			mongoCfg := &MongoReplConfig{
				Primary:       activeMembers[0],
				Count:         len(activeMembers),
				ReplSetId:     activeMembersMap[activeMembers[0]].ReplSetId,
				ReplSetName:   activeMembersMap[activeMembers[0]].ReplSetName,
				Members:       strings.Join(memberStr, ","),
				OpLogFirstSec: activeMembersMap[activeMembers[0]].OpLogFirstSec,
				OpLogFirstInc: activeMembersMap[activeMembers[0]].OpLogFirstInc,
				OpLogLastSec:  activeMembersMap[activeMembers[0]].OpLogLastSec,
				OpLogLasttInc: activeMembersMap[activeMembers[0]].OpLogLastInc,
			}
			mongoCfgStr, err := json.Marshal(mongoCfg)
			if err != nil {
				panic(err)
			}
			_, err = kv.Put(&capi.KVPair{
				Key:   "config/mongo",
				Value: mongoCfgStr,
			}, nil)
			if err != nil {
				panic(err)
			}
			monitorMode = true
			args := []string{"run", "-detach"}
			vars := map[string]string{
				"replica-count":   fmt.Sprintf("%d", len(activeMembers)),
				"replica-members": fmt.Sprintf("^(%s)$", strings.Join(activeMembers, "|")),
			}
			for k, v := range vars {
				args = append(args, "-var", fmt.Sprintf("%s=%s", k, v))
			}
			args = append(args, filepath.Join(os.Getenv("CMS_ROOT"), "jobs", "mongo", "mongodb.hcl"))
			log.Println("done. Deploying job...")
			cmd := exec.Command("nomad", args...)
			out, err := cmd.CombinedOutput()
			log.Println(err, string(out))
		}

	}

}
func strptr(s string) *string { return &s }
func sortMembersByOplog(members MongoStatusMap) []string {
	type kv struct {
		k string
		v *MongoStatus
	}
	var kvList []kv
	for k, v := range members {
		kvList = append(kvList, kv{k, v})
	}
	sort.Slice(kvList, func(i, j int) bool {
		o1 := kvList[i].v
		o2 := kvList[j].v

		if o1.OpLogLastSec == o2.OpLogLastSec {
			if o1.OpLogLastInc == o2.OpLogLastInc {
				return o1.NodeName > o2.NodeName
			}
			return o1.OpLogLastInc > o2.OpLogLastInc
		}
		return o1.OpLogLastSec > o2.OpLogLastSec
	})
	sorted := []string{}
	for _, k := range kvList {
		sorted = append(sorted, k.k)
	}
	return sorted
}

func watchStatus(ctx context.Context) rxgo.Observable {

	var lastIndex uint64 = 0
	waitTime := 5 * time.Second
	opts := &capi.QueryOptions{
		WaitIndex: lastIndex,
		WaitTime:  waitTime}
	opts = opts.WithContext(ctx)
	items := make(chan rxgo.Item)
	go func() {
		defer close(items)
		for {
			opts.WaitIndex = lastIndex
			pair, meta, err := kv.List(KV_PATH, opts)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Println("stopping watch")
					return
				}
				time.Sleep(time.Second * 2)
				continue
			}
			if meta != nil {
				if lastIndex == meta.LastIndex {
					continue
				}
				lastIndex = meta.LastIndex
			}
			curNodes := make(MongoStatusMap)
			for _, p := range pair {
				stat := &MongoStatus{}
				err = json.Unmarshal(p.Value, stat)
				if err != nil {
					log.Println("unmarsh err", err)
				} else {
					curNodes[stat.NodeName] = stat
				}
			}
			items <- rxgo.Of(curNodes)
		}
	}()
	return rxgo.FromChannel(items)
}

func configurer(ctx context.Context, mode bool) rxgo.Observable {
	watch := watchStatus(ctx)
	watchChan := watch.Observe()
	timer := time.NewTimer(time.Duration(math.MaxInt64))
	configured := mode
	var config MongoStatusMap
	configChan := make(chan rxgo.Item)
	debounceTime := time.Second * 10
	expired := mode
	go func() {

	loop:
		for {
			delay := 30 * time.Second
			select {
			case item, ok := <-watchChan:
				if !ok {
					log.Println("chan closed")
					watchChan = nil
					break loop
				}
				nodes := item.V.(MongoStatusMap)
				log.Println("status changed", configured, expired, nodes)
				if configured {
					if expired {
						expired = false
						configChan <- rxgo.Of(nodes)
						timer.Reset(debounceTime)
						config = nil
					} else {
						config = nodes
					}
					continue
				}
				if len(nodes) == int(CLUSTER_SIZE) {
					delay = 1 * time.Second
				} else if len(nodes) > 0 {
					log.Println("schedule delayed configuration")
					delay = 10 * time.Second
				} else {
					delay = time.Duration(math.MaxInt64)
				}
				timer.Reset(delay)
				config = nodes

			case <-timer.C:
				if !configured && len(config) > 0 {
					configured = true
					configChan <- rxgo.Of(config)
					timer.Reset(debounceTime)
					config = nil
				} else if config != nil {
					expired = false
					configChan <- rxgo.Of(config)
					timer.Reset(debounceTime)
					config = nil
				} else {
					expired = true
				}
			}
		}
	}()
	return rxgo.FromChannel(configChan)
}
