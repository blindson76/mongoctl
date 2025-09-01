package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/reactivex/rxgo/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ReplSetConfig struct {
	Config Replset `bson:"config"`
}

func mongoWrapper() {
	log.Println("start mongo wrapper task")
	obs := WatchKey[MongoReplConfig]("config/mongo").Observe()
	memberCfgVal, _, err := kv.Get(fmt.Sprintf("status/mongo/%s", NODE_NAME), nil)
	if err != nil {
		panic(err)
	}
	var memberCfg MongoStatus
	if err = json.Unmarshal(memberCfgVal.Value, &memberCfg); err != nil {
		panic(err)
	}

	//start mongo, check current config and status
	log.Println("starting mongod process")
	mongoSecretPath := filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "mongo.secret")
	log.Println("createing mongosecret file to", mongoSecretPath)
	if err := os.WriteFile(mongoSecretPath, []byte("MONGOSECRET"), 0); err != nil {
		panic(err)
	}

	rsName := MONGO_RSNAME
	var mongoProc *exec.Cmd
	var mongoCli *mongo.Client
	mongoProc = exec.Command("mongod", "--bind_ip", MONGO_ADDR, "--port", MONGO_PORT, "--replSet", rsName, "--dbpath", MONGO_DATA_DIR, "--keyFile", mongoSecretPath, "--auth")
	mongoProc.Stderr = os.Stderr
	mongoProc.Stdout = os.Stdout
	if err := mongoProc.Start(); err != nil {
		mongoProc = nil
		panic(err)
	}

	connectOpts := options.Client().SetHosts([]string{fmt.Sprintf("%s:%s", MONGO_ADDR, MONGO_PORT)}).SetServerSelectionTimeout(time.Second * 30).SetAuth(options.Credential{
		Username: "admin",
		Password: "123",
	}).SetDirect(true)
	log.Println("Connection options:", connectOpts)
	mongoCli, err = mongo.Connect(connectOpts)
	if err != nil {
		log.Println("mongo connect error", err)
		panic(err)
	}
	var activeReplCfg ReplSetConfig
	err = mongoCli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&activeReplCfg)
	if err == nil {
		log.Println("applying current replset config")
		memberCfg.ReplSetId = activeReplCfg.Config.Settings.ReplicaSetId.Hex()

	}

	isPrimary := false
	purgeDbRequired := false
	rsInitRequired := false
	for item := range obs {
		replConfig := item.V.(MongoReplConfig)
		isPrimary = replConfig.Primary == memberCfg.NodeName
		log.Println(replConfig, isPrimary, purgeDbRequired, rsInitRequired)

		if isPrimary {
			if memberCfg.ReplSetId == "000000000000000000000000" {
				rsInitRequired = true
			}

		} else if replConfig.ReplSetId != memberCfg.ReplSetId || replConfig.ReplSetName != memberCfg.ReplSetName ||
			replConfig.OpLogFirstSec > memberCfg.OpLogLastSec || replConfig.OpLogLastSec < memberCfg.OpLogFirstSec {
			purgeDbRequired = true
		}
		if purgeDbRequired {
			log.Println("purge dbpath", MONGO_DATA_DIR)
			if mongoProc != nil {
				log.Println("Killing active mongod proc")
				mongoProc.Process.Kill()
			}
			os.RemoveAll(MONGO_DATA_DIR)
			os.Mkdir(MONGO_DATA_DIR, 0)
		}

		if isPrimary {
			if rsInitRequired {
				log.Println("Initiating replicaset")
				members := []bson.D{}
				for _, member := range strings.Split(replConfig.Members, ",")[:1] {
					tokens := strings.Split(member, ":")
					id, _ := strconv.ParseInt(tokens[0], 10, 32)
					members = append(members, bson.D{
						{Key: "_id", Value: id},
						{Key: "host", Value: fmt.Sprintf("%s:%s", tokens[2], tokens[3])},
					})
				}
				replCmd := bson.D{{Key: "replSetInitiate", Value: bson.D{
					{Key: "_id", Value: rsName},
					{Key: "members", Value: members},
				}}}
				log.Println("replset config", replCmd)
				for i := 0; i < 5; i++ {
					if err := mongoCli.Database("admin").RunCommand(context.TODO(), replCmd).Err(); err != nil {
						log.Println("rsinit failed", err)
						time.Sleep(2 * time.Second)
					} else {
						log.Println("rsinitiated.")
						break
					}
				}
			}

		}
	}
}
func WatchKey[T any](path string) rxgo.Observable {

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
			value, meta, err := kv.Get(path, opts)
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
			var obj T
			err = json.Unmarshal(value.Value, &obj)
			if err != nil {
				items <- rxgo.Of(nil)
			} else {
				items <- rxgo.Of(obj)
			}
		}
	}()
	return rxgo.FromChannel(items)
}
