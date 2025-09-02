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
	var err error
	log.Println("start mongo wrapper task")
	obs := WatchKey[MongoReplConfig]("config/mongo").Observe()
	var decidedReplCfg MongoReplConfig
	var memberCfg MongoStatus
	if item, _, err := kv.Get("config/mongo", nil); err == nil && item != nil {
		if err = json.Unmarshal(item.Value, &decidedReplCfg); err != nil {
			log.Panicln("Unmarshal mongo config error", err)
		}
	} else {
		log.Panicln("Read mongo config error", err)
	}

	if item, _, err := kv.Get(fmt.Sprintf("status/mongo/%s", NODE_NAME), nil); err == nil && item != nil {
		if err = json.Unmarshal(item.Value, &memberCfg); err != nil {
			log.Panicln("Unmarshal mongo status error", err)
		}
	} else {
		log.Panicln("Read mongo status error", err)
	}

	isPrimary := decidedReplCfg.Primary == NODE_NAME
	log.Println("isPrimary node", isPrimary)

	if !isPrimary && checkWipeRequirment(&decidedReplCfg, &memberCfg) {
		if err = wipeDBPath(); err != nil {
			log.Panicln("Wipe db error", err)
		}
	}
	//start mongo, check current config and status
	log.Println("Starting mongod process")
	mongoSecretPath := filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "mongo.secret")
	log.Println("Createing mongosecret file to", mongoSecretPath)
	if err := os.WriteFile(mongoSecretPath, []byte("MONGOSECRET"), 0); err != nil {
		log.Panicln("Create mongo secret file error", err)
	}

	rsName := MONGO_RSNAME
	var mongoProc *exec.Cmd
	var mongoCli *mongo.Client
	mongoProc = exec.Command("mongod", "--bind_ip", MONGO_ADDR, "--port", MONGO_PORT, "--replSet", rsName, "--dbpath", MONGO_DATA_DIR, "--keyFile", mongoSecretPath, "--auth")
	mongoProc.Stderr = os.Stderr
	mongoProc.Stdout = os.Stdout
	if err := mongoProc.Start(); err != nil {
		mongoProc = nil
		log.Panic("Create mongod process error", err)
	}

	connectOpts := options.Client().SetHosts([]string{fmt.Sprintf("%s:%s", MONGO_ADDR, MONGO_PORT)}).SetServerSelectionTimeout(time.Second * 30).SetAuth(options.Credential{
		Username: "admin",
		Password: "123",
	}).SetDirect(true)
	log.Println("Connecting mongod", connectOpts)
	mongoCli, err = mongo.Connect(connectOpts)
	if err != nil {
		log.Panicln("Mongo connect error", err)
	}

	rsInitRequired := false
	for item := range obs {

		var activeReplCfg ReplSetConfig
		err = mongoCli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&activeReplCfg)
		if err == nil {
			log.Println("Applying current replset config")
			memberCfg.ReplSetId = activeReplCfg.Config.Settings.ReplicaSetId.Hex()

		}
		replConfig := item.V.(MongoReplConfig)
		log.Println(replConfig, isPrimary, rsInitRequired)

		if isPrimary {
			if memberCfg.ReplSetId == "" {
				rsInitRequired = true
			}

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
				initCmd := bson.D{{Key: "replSetInitiate", Value: bson.D{
					{Key: "_id", Value: rsName},
					{Key: "members", Value: members},
				}}}
				log.Println("replset init config", initCmd)
				initiated := false
				for i := 0; i < 5; i++ {
					if err := mongoCli.Database("admin").RunCommand(context.TODO(), initCmd).Err(); err != nil {
						log.Println("rsinit failed", err)
						time.Sleep(2 * time.Second)
					} else {
						log.Println("rsinitiated.")
						initiated = true
						break
					}
				}
				if initiated {
					log.Println("adding members")
					for _, member := range strings.Split(replConfig.Members, ",")[1:] {
						tokens := strings.Split(member, ":")
						id, _ := strconv.ParseInt(tokens[0], 10, 32)
						host := fmt.Sprintf("%s:%s", tokens[2], tokens[3])
						log.Println("adding members", id, host)
						if err := addMember(mongoCli, id, host); err != nil {
							log.Println("member added failed", id, host)
						} else {
							log.Println("OK.")
						}
						time.Sleep(2 * time.Second)
					}

				}
			}
			log.Println("Checking reconf requirment")
			checkReplMembers(mongoCli, &replConfig)

		}
	}
}

func checkReplMembers(cli *mongo.Client, replCfg *MongoReplConfig) error {
	var currentReplCfg ReplSetConfig
	err := cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&currentReplCfg)
	if err != nil {
		return err
	}
	currMemberMap := make(map[int64]string)
	desiredMemberMap := make(map[int64]string)

	reconfRequired := false

	for _, hostStr := range strings.Split(replCfg.Members, ",") {
		tokens := strings.Split(hostStr, ":")
		nodeId, _ := strconv.ParseInt(tokens[0], 10, 32)
		host := fmt.Sprintf("%s:%s", tokens[2], tokens[3])
		desiredMemberMap[nodeId] = host
		log.Println("Desired:", nodeId, host)
	}

	for _, member := range currentReplCfg.Config.Members {
		currMemberMap[int64(member.ID)] = member.Host
		log.Println("Current:", member.ID, member.Host)
		if _, ok := desiredMemberMap[int64(member.ID)]; !ok {
			reconfRequired = true
			break
		}
	}
	reconfRequired = reconfRequired || len(currMemberMap) == len(desiredMemberMap)
	if reconfRequired {
		log.Println("reconf")
		var replConfig ReplSetConfig
		err := cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&replConfig)
		if err != nil {
			return err
		}
		replConfig.Config.Members = nil
		for k, v := range desiredMemberMap {
			newMember := Member{
				ID:   int(k),
				Host: v,
			}
			replConfig.Config.Members = append(replConfig.Config.Members, newMember)
		}
		replConfig.Config.Version += 1
		cmd := bson.D{
			{Key: "replSetReconfig", Value: replConfig.Config},
			{Key: "force", Value: true},
		}
		log.Println("reconf cmd", cmd)

		err = cli.Database("admin").RunCommand(context.TODO(), cmd).Err()
		return err

	}
	return nil
}

func addMember(cli *mongo.Client, id int64, host string) error {
	var replConfig ReplSetConfig
	err := cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&replConfig)
	if err != nil {
		return err
	}
	newMember := Member{
		ID:   int(id),
		Host: host,
	}
	replConfig.Config.Members = append(replConfig.Config.Members, newMember)
	replConfig.Config.Version += 1
	cmd := bson.D{
		{Key: "replSetReconfig", Value: replConfig.Config},
		{Key: "force", Value: true},
	}

	err = cli.Database("admin").RunCommand(context.TODO(), cmd).Err()
	return err

}

func checkWipeRequirment(replConfig *MongoReplConfig, memberCfg *MongoStatus) bool {
	log.Println("Checking wipre requirments")
	log.Println("replcfg", replConfig)
	log.Println("memberCfg", memberCfg)
	if memberCfg.ReplSetId == "" {
		log.Println("replset nt initiated")
		return false
	} else if replConfig.ReplSetId != memberCfg.ReplSetId {
		log.Println("replset id different")
		return true
	} else if replConfig.ReplSetName != memberCfg.ReplSetName {
		log.Println("replset name different")
		return true
	} else if replConfig.OpLogFirstSec > memberCfg.OpLogLastSec {
		log.Println("oplogs too far")
		return true
	}
	return false
}
func wipeDBPath() error {

	log.Println("wipe dbpath", MONGO_DATA_DIR)
	if err := os.RemoveAll(MONGO_DATA_DIR); err != nil {
		return err
	}
	os.Mkdir(MONGO_DATA_DIR, 0)
	return nil
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
