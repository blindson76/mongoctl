package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoWrapper struct {
	cli      *mongo.Client
	nodeID   string
	nodeName string
	mongo    *MongodMgm
	mongoCli *MongoClient
}

func (w *MongoWrapper) GetMemberStatus() (*MongoStatus, error) {
	mongoStatus := &MongoStatus{
		NodeId:    NODE_ID,
		NodeName:  NODE_NAME,
		MongoAddr: MONGO_ADDR,
		MongoPort: MONGO_PORT,
	}
	status, err := w.mongoCli.ReplSetGetStatus()
	if err != nil {
		log.Println("status err")
		return nil, err
	}
	log.Println("ReplSetGetStatus", status)
	if status != nil {
		mongoStatus.OpLogLastInc = status.Optimes.AppliedOpTime.Ts.I
		mongoStatus.OpLogLastSec = status.Optimes.AppliedOpTime.Ts.T
		mongoStatus.Term = status.Term
	}
	config, err := w.mongoCli.ReplSetGetConfig()
	if err != nil {
		return nil, err
	}

	log.Println("ReplSetGetConfig", config)
	if config != nil {
		log.Println("config", config)
		mongoStatus.ReplSetId = config.Config.Settings.ReplicaSetId.Hex()
		mongoStatus.ReplSetName = config.Config.ID
		members := []string{}
		for _, m := range config.Config.Members {
			members = append(members, fmt.Sprintf("%d::%s", m.ID, m.Host))
		}
		mongoStatus.Members = strings.Join(members, ",")
	}

	return mongoStatus, nil

}
func (w *MongoWrapper) GetMongoStatus() (*MongoStatus, error) {
	item, _, err := kv.Get(fmt.Sprintf("status/mongo/%s", NODE_NAME), nil)
	if err != nil {
		return nil, err
	}
	var status MongoStatus
	if json.Unmarshal(item.Value, &status) != nil {
		return nil, err
	}
	return &status, nil
}
func (w *MongoWrapper) Start() {
	configObserver := ConsulWatchKey[MongoReplConfig]("config/mongo")
	w.mongo = &MongodMgm{
		BindIp:   MONGO_ADDR,
		BindPort: MONGO_PORT,
		DBPath:   MONGO_DATA_DIR,
		ReplSet:  MONGO_RSNAME,
		Auth:     true,
		KeyFile:  MONGO_SECRET_FILE,
		OnExit: func(state *os.ProcessState) {
			log.Println("Mongod exited", state)
		},
	}
	if err := w.mongo.Start(); err != nil {
		log.Panicln("Mongo start error", err)
	}
	w.mongoCli = w.mongo.Client()
	err := w.mongoCli.ConnectWithOptions(func(opts *options.ClientOptions) *options.ClientOptions {
		return opts.SetDirect(true).SetAuth(options.Credential{
			Username: "admin",
			Password: "123",
		}).SetReplicaSet("")

	})
	if err != nil {
		log.Panicln("Mongo client connect error", err)
	}
	if w.mongoCli.cli.Ping(context.TODO(), nil) != nil {
		log.Panicln("Ping error", err)
	}
	time.Sleep(5 * time.Second)
	for mongoConfigItem := range configObserver.Observe() {
		mongoConfig := mongoConfigItem.V.(MongoReplConfig)
		isPrimary := mongoConfig.Primary == NODE_NAME

		memberStatus, err := w.GetMemberStatus()
		if err != nil {
			log.Println("fetc mongostatus error", err)
			switch e := err.(type) {
			case mongo.CommandError:
				if e.Name == "InvalidReplicaSetConfig" && isPrimary {
					log.Println("stale cfg. try reconfigure")

					desiredMember := w.ParseMembers(mongoConfig.Members)
					cfg, err := w.mongoCli.ReplSetGetConfig()
					newMembers := []Member{}
					for _, m := range desiredMember {
						newMembers = append(newMembers, m)
					}
					cfg.Config.Members = newMembers
					err = w.mongoCli.ReplSetReconfig(&cfg.Config)

					log.Println("reconf", err)
					if err != nil {
						time.Sleep(3 * time.Second)
						log.Println("stale conf recovery done")
						continue
					}
				}
			}
		}
		log.Println("mongoConfig", mongoConfig, "memberStatus", memberStatus)

		//stop if mongod running
		if !isPrimary && w.checkWipeRequirment(mongoConfig, memberStatus) {
			log.Println("Stopping mongod to wipe")
			pState := w.mongo.ShutdownWithTimeout(10 * time.Second)
			log.Println("mongo exited?", pState)
			if err := w.wipeDB(); err != nil {
				panic(err)

			}
			if err := w.mongo.Start(); err != nil {
				log.Panicln("Mongo start error", err)
			}
			w.mongoCli = w.mongo.Client()
			err := w.mongoCli.ConnectWithOptions(func(opts *options.ClientOptions) *options.ClientOptions {
				return opts.SetDirect(true).SetAuth(options.Credential{
					Username: "admin",
					Password: "123",
				}).SetReplicaSet("")

			})
			if err != nil {
				log.Panicln("Mongo client connect error", err)
			}
		}
		//start if mongo not running
		log.Println(isPrimary, mongoConfig.Primary, memberStatus.NodeName)
		if isPrimary {
			if err := w.configPrimary(mongoConfig, memberStatus); err != nil {
				log.Println("Primary configuration error", err)
			}
		}
		log.Println("Done")
		time.Sleep(50000 * time.Second)
	}
	/*
		load mongo config
		load current status
		check if is priamry or not
		if secondary
			check if wipe requirment
				if true: wipe db

	*/

}

func (w *MongoWrapper) configPrimary(mongoConfig MongoReplConfig, memberStatus *MongoStatus) error {
	members := []Member{}

	for _, member := range strings.Split(mongoConfig.Members, ",") {
		tokens := strings.Split(member, ":")
		id, _ := strconv.ParseInt(tokens[0], 10, 32)
		host := fmt.Sprintf("%s:%s", tokens[2], tokens[3])
		members = append(members, Member{ID: int(id), Host: host})
	}
	hasStaleConfig := memberStatus.ReplSetId == "" && mongoConfig.ReplSetId != ""
	log.Println("Stale config", hasStaleConfig)
	if memberStatus.ReplSetName == "" && !hasStaleConfig {
		log.Println("Initiating replset", MONGO_RSNAME, members[0:1])
		if err := w.mongoCli.ReplSetInitiate(MONGO_RSNAME, members[0:1]); err != nil {
			log.Panicln("Initiating primary only member failed", err)
		}
		if len(members) > 1 {
			log.Println("waiting a while before add secondaries")
			time.Sleep(3 * time.Second)
			log.Println("Adding secondaries")
			if err := w.mongoCli.AddMember(members[1:]); err != nil {
				log.Panic("Adding secondary members error", err)
			}
		}
	} else {
		currentMembers := w.ParseMembers(memberStatus.Members)
		desiredMember := w.ParseMembers(mongoConfig.Members)
		reconfNeeded := false
		if hasStaleConfig {
			reconfNeeded = true
		} else if len(currentMembers) != len(desiredMember) {
			log.Println("reconf due to stale config")
			reconfNeeded = true
		} else {
			for dk, dv := range desiredMember {
				cv, ok := currentMembers[dk]
				if !ok {
					log.Println(dv.Host, "not found in current replset")
					reconfNeeded = true
					break
				} else {
					if cv.Host != dv.Host {
						log.Println(fmt.Sprintf("%d:%s in current cfg will change to %s", dk, cv.Host, dv.Host))
						reconfNeeded = true
						break
					}
				}
			}
		}
		if reconfNeeded {
			cfg, err := w.mongoCli.ReplSetGetConfig()
			if err != nil {
				log.Println("couldnt get replconfig while reconfiguring", err)
				return err
			}
			newMembers := []Member{}
			for _, m := range desiredMember {
				newMembers = append(newMembers, m)
			}
			if hasStaleConfig {
				cfg = &ReplSetConfig{
					Config: Replset{
						ID: MONGO_RSNAME,
					},
				}
			}
			cfg.Config.Members = newMembers
			if err := w.mongoCli.ReplSetReconfig(&cfg.Config); err != nil {
				log.Println("Reconfiguring replset error with", newMembers, err)
				return err
			}
		}

		log.Println("Checking reconfiguration", currentMembers, desiredMember)
	}
	return nil
}
func (w *MongoWrapper) ParseMembers(membersStr string) map[int]Member {
	members := make(map[int]Member)
	for _, m := range strings.Split(membersStr, ",") {
		tokens := strings.Split(m, ":")
		if len(tokens) < 4 {
			continue
		}
		id, _ := strconv.ParseInt(tokens[0], 10, 32)
		members[int(id)] = Member{ID: int(id), Host: fmt.Sprintf("%s:%s", tokens[2], tokens[3])}
	}
	return members
}
func (w *MongoWrapper) wipeDB() error {

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
func (w *MongoWrapper) checkWipeRequirment(replConfig MongoReplConfig, memberCfg *MongoStatus) bool {
	log.Println("Checking wipee requirments")
	log.Println("replcfg", replConfig)
	log.Println("memberCfg", memberCfg)
	if memberCfg.ReplSetName == "" {
		log.Println("replset not initiated")
		return false
	} else if replConfig.ReplSetId != memberCfg.ReplSetId && replConfig.ReplSetId != "" {
		log.Println("replset id different")
		return true
	} else if replConfig.ReplSetName != memberCfg.ReplSetName {
		log.Println("replset name different")
		return true
	} else if replConfig.OpLogFirstSec > uint32(memberCfg.OpLogLastInc) {
		log.Println("oplogs too far")
		return true
	}
	log.Println("wipe not required id different")
	return false
}

/*

- load

*/

func mongoWrapper() {

	wrap := &MongoWrapper{}
	wrap.Start()
	if true {
		return
	}
	var err error
	log.Println("start mongo wrapper task")
	configObserver := ConsulWatchKey[MongoReplConfig]("config/mongo")
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
	for item := range configObserver.Observe() {

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
			err = checkReplMembers(mongoCli, &replConfig)
			if err != nil {
				log.Panicln("check replmember err", err)
			}
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
		if err != nil {
			log.Println("reconf err", err)
		}
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
