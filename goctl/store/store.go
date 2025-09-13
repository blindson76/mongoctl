package store

import (
	"encoding/json"
	"fmt"
	capi "github.com/hashicorp/consul/api"
	"log"
	"path"
	"time"
)

type Unique[T any] interface {
	GetId() string
}

type Orderable[T any] interface {
	Less(other T) bool
}

type CandidateReportType[C any] interface {
	Orderable[C]
	Unique[C]
}
type HealtStatusType[H any] interface {
	Orderable[H]
	Unique[H]
	IsHealthy() bool
}
type ReplicaSetSpecType[S any] interface {
	ApplyConfig() error
}

type Provider[C CandidateReportType[C], S ReplicaSetSpecType[S], H HealtStatusType[H]] interface {
	PutCandidateReport(id string, Val *C) error
	WatchCandidateReports() <-chan []C
	UpdateHealthStatus(id string, status H) error
	WatchHealthStatus() <-chan []H
	UpdateReplSetConfig(cfg *S) error
	WatchReplSetConfig() <-chan S
}
type ConsulStore[C CandidateReportType[C], S ReplicaSetSpecType[S], H HealtStatusType[H]] struct {
	Provider[C, S, H]
	candidateReportPath string
	healthStatusPath    string
	replSetConfigPath   string
	cli                 *capi.Client
}
type ConsulStoreConfig struct {
	CandidateReportPath string
	HealthStatusPath    string
	ReplSetConfigPath   string
}

func NewConsulStore[C CandidateReportType[C], S ReplicaSetSpecType[S], H HealtStatusType[H]](consulAddr string, config ConsulStoreConfig) (*ConsulStore[C, S, H], error) {
	store := &ConsulStore[C, S, H]{
		candidateReportPath: config.CandidateReportPath,
		healthStatusPath:    config.HealthStatusPath,
		replSetConfigPath:   config.ReplSetConfigPath,
	}
	conf := capi.DefaultConfig()
	conf.Address = consulAddr
	cli, err := capi.NewClient(conf)
	if err != nil {
		return nil, err
	}
	store.cli = cli
	return store, err
}
func (c ConsulStore[C, S, H]) PutCandidateReport(id string, val *C) error {
	cli := c.cli
	sess := cli.Session()
	kv := cli.KV()
	agent := cli.Agent()
	sessionName := fmt.Sprintf("%s.mongo-status", id)
	var sessionId string
	statusVal, _, err := kv.Get(fmt.Sprintf("%s/%s", c.candidateReportPath, id), nil)
	if err != nil {
		return err
	}
	if statusVal != nil {
		sessionId = statusVal.Session
	}
	if sessionId == "" {
		//creating session
		log.Println("creating session")
		var check string

	tryloop:
		for i := 0; i < 5; i++ {
			checks, err := agent.Checks()
			if err != nil {
				return err
			}

			for k, v := range checks {
				if v.Name == "Nomad Client HTTP Check" && v.Status == "passing" {
					log.Println(k, v.Name)
					check = v.CheckID
					break tryloop
				}
			}
			log.Println("check failed")
			time.Sleep(time.Second * 2)
		}
		if check == "" {
			return fmt.Errorf("Check not found")
		}

		session, _, err := sess.Create(&capi.SessionEntry{
			Behavior:      capi.SessionBehaviorDelete,
			ServiceChecks: []capi.ServiceCheck{{ID: check}},
			Name:          sessionName,
		},
			nil)
		if err != nil {
			return err
		}
		sessionId = session
	}
	log.Println("session", sessionId)
	statusStr, err := json.Marshal(val)
	if err != nil {
		return err
	}
	log.Println("Mongo status:", string(statusStr))
	_, _, err = kv.Acquire(&capi.KVPair{
		Key:     fmt.Sprintf("%s/%s", c.candidateReportPath, id),
		Session: sessionId,
		Value:   statusStr,
	}, nil)
	return err
}

func (c ConsulStore[C, S, H]) WatchCandidateReports() <-chan []C {
	out := make(chan []C, 1)
	go func() {
		kv := c.cli.KV()
		var lastIdx uint64
		for {
			pairs, meta, err := kv.List(c.candidateReportPath, &capi.QueryOptions{
				WaitIndex: lastIdx,
				// WaitTime can be set if desired; default server max wait applies.
			})
			if err != nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if meta != nil {
				lastIdx = meta.LastIndex
			}
			var items []C
			for _, p := range pairs {
				var v C
				if err := json.Unmarshal(p.Value, &v); err == nil {
					items = append(items, v)
				}
			}
			out <- items
		}
	}()
	return out
}

func (c ConsulStore[C, S, H]) UpdateHealthStatus(id string, status H) error {
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	key := path.Join(c.healthStatusPath, id)
	_, err = c.cli.KV().Put(&capi.KVPair{Key: key, Value: data}, nil)
	return err
}

func (c ConsulStore[C, S, H]) WatchHealthStatus() <-chan []H {
	out := make(chan []H, 1)
	go func() {
		kv := c.cli.KV()
		var lastIdx uint64
		for {
			pairs, meta, err := kv.List(c.healthStatusPath, &capi.QueryOptions{
				WaitIndex: lastIdx,
			})
			if err != nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if meta != nil {
				lastIdx = meta.LastIndex
			}
			var items []H
			for _, p := range pairs {
				var v H
				if err := json.Unmarshal(p.Value, &v); err == nil {
					items = append(items, v)
				}
			}
			out <- items
		}
	}()
	return out
}

func (c ConsulStore[C, S, H]) UpdateReplSetConfig(cfg *S) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = c.cli.KV().Put(&capi.KVPair{Key: c.replSetConfigPath, Value: data}, nil)
	return err
}

func (c ConsulStore[C, S, H]) WatchReplSetConfig() <-chan S {
	out := make(chan S, 1)
	go func() {
		kv := c.cli.KV()
		var lastIdx uint64
		for {
			pair, meta, err := kv.Get(c.replSetConfigPath, &capi.QueryOptions{
				WaitIndex: lastIdx,
			})
			if err != nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if meta != nil {
				lastIdx = meta.LastIndex
			}
			if pair == nil || len(pair.Value) == 0 {
				// No config yet; keep waiting.
				continue
			}
			var cfg S
			if err := json.Unmarshal(pair.Value, &cfg); err != nil {
				// Skip malformed value; keep waiting.
				continue
			}
			out <- cfg
		}
	}()
	return out
}
