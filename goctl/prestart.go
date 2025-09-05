package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	capi "github.com/hashicorp/consul/api"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func wipeDB() error {
	log.Println("wiping dbpath", MONGO_DATA_DIR)
	info, err := os.Stat(MONGO_DATA_DIR)
	isExists := err != nil || info.IsDir()
	if !isExists {
		log.Println("Directory not exists")
		return nil
	}
	entries, err := os.ReadDir(MONGO_DATA_DIR)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryPath := filepath.Join(MONGO_DATA_DIR, entry.Name())
		err := os.RemoveAll(entryPath)
		if err != nil {
			log.Println("error while removing", entryPath, err)
			return err
		}
	}
	return nil
}
func prestart_job() {
	if err := prestart_task(); err != nil {
		log.Println("prestart failed:", err)
		if wipeDB() == nil {
			prestart_task()
		}
	}
}
func prestart_task() error {

	mongod := MongodMgm{
		DBPath:   MONGO_DATA_DIR,
		BindIp:   MONGO_LOCAL_ADDR,
		BindPort: MONGO_LOCAL_PORT,
		OnExit: func(state *os.ProcessState) {
			log.Println("mongod exitedd", state)
		},
	}
	err := mongod.Start()
	if err != nil {
		return err
	}
	mongoCli := mongod.Client()

	if err = mongoCli.ConnectWithOptions(func(opts *options.ClientOptions) *options.ClientOptions {
		return opts.SetConnectTimeout(15 * time.Second).SetServerSelectionTimeout(15 * time.Second).SetDirect(true)
	}); err != nil {
		return err
	}

	userExist, err := mongoCli.HasUser("admin")
	if err != nil {
		return err
	}
	if userExist {
		log.Println("user already exist")
	} else {
		if err := mongoCli.CreateUser("admin", "123"); err != nil {
			return fmt.Errorf("Failed to create user: %s", err)
		}

	}

	status := &MongoStatus{
		NodeId:    NODE_ID,
		NodeName:  NODE_NAME,
		MongoAddr: MONGO_ADDR,
		MongoPort: MONGO_PORT,
	}
	var oplogFirst OpLog
	var oplogLast OpLog
	replset, err := mongoCli.ReplSetGetConfigOffline()
	if err != nil {
		log.Println("not found", replset, err, err.Error() == "NoReplicationEnabled")
	}
	log.Println("repLSet", replset)
	if replset != nil && replset.Config.ID != "" {
		log.Println("has replicaset config")

		opLogs, err := mongoCli.GetOplogWindow()
		if err != nil {
			return err
		} else {
			oplogFirst.Ts = opLogs[0].Ts
			oplogLast.Ts = opLogs[1].Ts
			hosts := make([]string, len(replset.Config.Members))
			for i, m := range replset.Config.Members {
				hosts[i] = fmt.Sprintf("%d::%s", m.ID, m.Host)
			}
			status.ReplSetId = replset.Config.Settings.ReplicaSetId.Hex()
			status.ReplSetName = replset.Config.ID
			status.Members = strings.Join(hosts, ",")
			status.Term = replset.Config.Term
			status.OpLogFirstSec = oplogFirst.Ts.T
			status.OpLogFirstInc = oplogFirst.Ts.I
			status.OpLogLastSec = oplogLast.Ts.T
			status.OpLogLastInc = oplogLast.Ts.I
		}
	}

	conf := capi.DefaultConfig()
	conf.Address = CONSUL_HTTP_ADDR
	cli, err := capi.NewClient(conf)
	if err != nil {
		return err
	}
	sess := cli.Session()
	kv := cli.KV()
	agent := cli.Agent()
	sessionName := fmt.Sprintf("%s.mongo-status", NODE_NAME)
	var sessionId string

	statusVal, _, err := kv.Get(fmt.Sprintf("%s/%s", KV_PATH, NODE_NAME), nil)
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
	statusStr, err := json.Marshal(status)
	if err != nil {
		return err
	}
	log.Println("Mongo status:", string(statusStr))
	done, _, err := kv.Acquire(&capi.KVPair{
		Key:     fmt.Sprintf("%s/%s", KV_PATH, NODE_NAME),
		Session: sessionId,
		Value:   statusStr,
	}, nil)
	log.Println("Done", done)
	log.Println("Exit:", mongod.ShutdownWithTimeout(3*time.Second))
	return nil
}
