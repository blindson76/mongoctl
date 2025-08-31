package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	capi "github.com/hashicorp/consul/api"
	napi "github.com/hashicorp/nomad/api"
	"github.com/reactivex/rxgo/v2"
)

var (
	ncli *napi.Client
)

type MongoReplConfig struct {
	Count       int    `json:"count"`
	Primary     string `json:"primary"`
	Members     string `json:"members"`
	ReplSetId   string `json:"replSetId"`
	ReplSetName string `json:"repLSetName"`
}

func controllerJob() {
	ncli, err := napi.NewClient(&napi.Config{
		Address: NOMAD_ADDR,
	})
	if err != nil {
		panic(err)
	}
	var activeMembers []string
	activeMembersMap := make(MongoStatusMap)

	config := configurer(context.Background())
	for cfg := range config.Observe() {
		log.Println("new conf")
		nodes := cfg.V.(MongoStatusMap)
		sorted := sortMembersByOplog(nodes)
		last := int(math.Min(float64(len(sorted)), 3))
		selected := sorted[0:last]
		selectedMap := make(MongoStatusMap)

		var removedMembers []string
		var addedMembers []string

		for _, nodeId := range selected {
			selectedMap[nodeId] = nodes[nodeId]
			if _, ok := activeMembersMap[nodeId]; !ok {
				addedMembers = append(addedMembers, nodeId)
				activeMembersMap[nodeId] = nodes[nodeId]
				activeMembers = append(activeMembers, nodeId)
			}

		}

		for nodeId, _ := range activeMembersMap {
			if _, ok := selectedMap[nodeId]; !ok {
				removedMembers = append(removedMembers, nodeId)
				delete(activeMembersMap, nodeId)
			}
		}

		log.Println("added", addedMembers)
		log.Println("removed", removedMembers)
		activeMembers = selected
		log.Println(activeMembers, activeMembersMap)

		mongoCfg := &MongoReplConfig{
			Primary:     activeMembers[0],
			Count:       len(activeMembers),
			ReplSetId:   activeMembersMap[activeMembers[0]].ReplSetId,
			ReplSetName: activeMembersMap[activeMembers[0]].ReplSetName,
			Members:     strings.Join(activeMembers, ","),
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

		var addedWG sync.WaitGroup
		nomadNodes, _, err := ncli.Nodes().List(nil)
		if err != nil {
			panic(err)
		}
		nodesIdMap := make(map[string]string)
		for i, nn := range nomadNodes {
			log.Println(i, nn.Name)
			nodesIdMap[nn.Name] = nn.ID
		}
		for _, nodeId := range addedMembers {
			addedWG.Add(1)
			go func() {
				defer addedWG.Done()
				log.Println(nodeId)
				_, err := ncli.Nodes().Meta().Apply(&napi.NodeMetaApplyRequest{
					NodeID: nodesIdMap[nodeId],
					Meta: map[string]*string{
						"mongo.role": strptr("true"),
					},
				}, nil)
				if err != nil {
					log.Println("updatemeta err", err)
				}

			}()
		}
		addedWG.Wait()
		log.Println("done added")
		for _, nodeId := range removedMembers {
			addedWG.Add(1)
			go func() {
				defer addedWG.Done()
				log.Println(nodeId)
				_, err := ncli.Nodes().Meta().Apply(&napi.NodeMetaApplyRequest{
					NodeID: nodesIdMap[nodeId],
					Meta: map[string]*string{
						"mongo.role": nil,
					},
				}, nil)
				if err != nil {
					log.Println("updatemeta err", err)
				}

			}()
		}
		addedWG.Wait()
		log.Println("done ")

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
			if o1.OpLogLasttInc == o2.OpLogLasttInc {
				return o1.NodeId > o2.NodeId
			}
			return o1.OpLogLasttInc > o2.OpLogLasttInc
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
					curNodes[stat.NodeId] = stat
				}
			}
			items <- rxgo.Of(curNodes)
		}
	}()
	return rxgo.FromChannel(items)
}

func configurer(ctx context.Context) rxgo.Observable {
	watch := watchStatus(ctx)
	watchChan := watch.Observe()
	timer := time.NewTimer(time.Duration(math.MaxInt64))
	configured := false
	var config MongoStatusMap
	configChan := make(chan rxgo.Item)
	debounceTime := time.Second * 10
	expired := false
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
