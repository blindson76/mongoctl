package replset

import (
	"context"
	"example.com/goctl/store"
	"example.com/goctl/util"
	"fmt"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type MongoCandidateReport struct {
	NodeId        string
	NodeName      string
	ReplSetId     string
	ReplSetName   string
	Members       string
	Term          int64
	OpLogFirstSec uint32
	OpLogFirstInc uint32
	OpLogLastSec  uint32
	OpLogLastInc  uint32
	MongoAddr     string
	MongoPort     string
}
type MongoConfig struct {
	Addr      string
	Port      string
	LocalAddr string
	LocalPort string
	DBPath    string
	RSName    string
	NodeID    string
	NodeName  string
	KeyFile   string
}

func NewMongoController(mongoCfg MongoConfig, str store.Provider[MongoCandidateReport, MongoReplSetSpec, MongoHealthStatus]) *MongoController {
	mc := &MongoController{
		cfg: mongoCfg,
	}
	mc.replicaSetController = replicaSetController[MongoCandidateReport, MongoReplSetSpec, MongoHealthStatus]{
		collector: mc,
		store:     str,
	}
	return mc
}
func (m MongoCandidateReport) Less(o MongoCandidateReport) bool {

	if m.OpLogLastSec == o.OpLogLastSec {
		if m.OpLogLastInc == o.OpLogLastInc {
			return m.NodeName > o.NodeName
		}
		return m.OpLogLastInc > o.OpLogLastInc
	}
	return m.OpLogLastSec > o.OpLogLastSec
}

func (m MongoCandidateReport) GetId() string {
	return m.NodeId
}

type MongoHealthStatus struct {
	NodeId   string
	NodeName string
	Status   string
}

func (m MongoHealthStatus) Less(o MongoHealthStatus) bool {
	//TODO implement me
	return false
}

func (m MongoHealthStatus) GetId() string {
	return m.NodeName
}

func (m MongoHealthStatus) IsHealthy() bool {
	return true
}

type MongoReplSetSpec struct {
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

func (m MongoReplSetSpec) ApplyConfig() error {
	//TODO implement me
	panic("implement me")
}

type MongoController struct {
	cfg MongoConfig
	replicaSetController[MongoCandidateReport, MongoReplSetSpec, MongoHealthStatus]
	mongo    *util.MongodMgm
	mongoCli *util.MongoClient
}

func (m MongoController) generateReplConfig(candidates []MongoCandidateReport) *MongoReplSetSpec {
	memberStr := []string{}
	for _, m := range candidates {
		memberStr = append(memberStr, fmt.Sprintf("%s:%s:%s:%s", m.NodeId, m.NodeName, m.MongoAddr, m.MongoPort))
	}

	mongoCfg := &MongoReplSetSpec{
		Primary:       candidates[0].NodeName,
		Count:         len(candidates),
		ReplSetId:     candidates[0].ReplSetId,
		ReplSetName:   candidates[0].ReplSetName,
		Members:       strings.Join(memberStr, ","),
		OpLogFirstSec: candidates[0].OpLogFirstSec,
		OpLogFirstInc: candidates[0].OpLogFirstInc,
		OpLogLastSec:  candidates[0].OpLogLastSec,
		OpLogLasttInc: candidates[0].OpLogLastInc,
	}
	return mongoCfg
}
func (m MongoController) collect() (*MongoCandidateReport, error) {
	mongod := util.MongodMgm{
		DBPath:   m.cfg.DBPath,
		BindIp:   m.cfg.LocalAddr,
		BindPort: m.cfg.LocalPort,
		OnExit: func(state *os.ProcessState) {
			log.Println("mongod exitedd", state)
		},
	}
	err := mongod.Start()
	if err != nil {
		return nil, err
	}
	mongoCli := mongod.Client()

	if err = mongoCli.ConnectWithOptions(func(opts *options.ClientOptions) *options.ClientOptions {
		return opts.SetConnectTimeout(15 * time.Second).SetServerSelectionTimeout(15 * time.Second).SetDirect(true)
	}); err != nil {
		return nil, err
	}

	userExist, err := mongoCli.HasUser("admin")
	if err != nil {
		return nil, err
	}
	if userExist {
		log.Println("user already exist")
	} else {
		if err := mongoCli.CreateUser("admin", "123"); err != nil {
			return nil, fmt.Errorf("Failed to create user: %s", err)
		}

	}

	status := &MongoCandidateReport{
		NodeId:    viper.GetString("node.id"),
		NodeName:  viper.GetString("node.name"),
		MongoAddr: m.cfg.Addr,
		MongoPort: m.cfg.Port,
	}
	var oplogFirst util.OpLog
	var oplogLast util.OpLog
	replset, err := mongoCli.ReplSetGetConfigOffline(m.cfg.RSName)
	if err != nil {
		log.Println("not found", replset, err, err.Error() == "NoReplicationEnabled")
	}
	log.Println("repLSet", replset)
	if replset != nil && replset.Config.ID != "" {
		log.Println("has replicaset config")

		opLogs, err := mongoCli.GetOplogWindow()
		if err != nil {
			return nil, err
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

	log.Println("Exiting mongod:", mongod.ShutdownWithTimeout(3*time.Second))
	return status, nil
}

func (m MongoController) memberTask(configChan <-chan MongoReplSetSpec) <-chan MongoHealthStatus {
	log.Println("member controller")
	m.mongo = &util.MongodMgm{
		BindIp:   m.cfg.Addr,
		BindPort: m.cfg.Port,
		DBPath:   m.cfg.DBPath,
		ReplSet:  m.cfg.RSName,
		Auth:     true,
		KeyFile:  m.cfg.KeyFile,
		OnExit: func(state *os.ProcessState) {
			log.Println("Mongod exited", state)
		},
	}
	if err := m.mongo.Start(); err != nil {
		log.Panicln("Mongo start error", err)
	}
	m.mongoCli = m.mongo.Client()
	err := m.mongoCli.ConnectWithOptions(func(opts *options.ClientOptions) *options.ClientOptions {
		return opts.SetDirect(true).SetAuth(options.Credential{
			Username: "admin",
			Password: "123",
		}).SetReplicaSet("")

	})
	if err != nil {
		log.Panicln("Mongo client connect error", err)
	}
	if m.mongoCli.Cli.Ping(context.TODO(), nil) != nil {
		log.Panicln("Ping error", err)
	}
	time.Sleep(5 * time.Second)
	for mongoConfigItem := range configChan {
		mongoConfig := mongoConfigItem
		isPrimary := mongoConfig.Primary == m.cfg.NodeName

		memberStatus, err := m.GetMemberStatus()
		if err != nil {
			log.Println("fetc mongostatus error", err)
			switch e := err.(type) {
			case mongo.CommandError:
				if e.Name == "InvalidReplicaSetConfig" && isPrimary {
					log.Println("stale cfg. try reconfigure")

					desiredMember := m.ParseMembers(mongoConfig.Members)
					cfg, err := m.mongoCli.ReplSetGetConfig()
					newMembers := []util.Member{}
					for _, m := range desiredMember {
						newMembers = append(newMembers, m)
					}
					cfg.Config.Members = newMembers
					err = m.mongoCli.ReplSetReconfig(&cfg.Config)

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
		if !isPrimary && m.checkWipeRequirment(mongoConfig, memberStatus) {
			log.Println("Stopping mongod to wipe")
			pState := m.mongo.ShutdownWithTimeout(10 * time.Second)
			log.Println("mongo exited?", pState)
			if err := m.wipeDB(); err != nil {
				panic(err)

			}
			if err := m.mongo.Start(); err != nil {
				log.Panicln("Mongo start error", err)
			}
			m.mongoCli = m.mongo.Client()
			err := m.mongoCli.ConnectWithOptions(func(opts *options.ClientOptions) *options.ClientOptions {
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
			if err := m.configPrimary(mongoConfig, memberStatus); err != nil {
				log.Println("Primary configuration error", err)
			}
		}
		log.Println("Done")
		time.Sleep(50000 * time.Second)
	}
	return nil

}

func (m *MongoController) configPrimary(mongoConfig MongoReplSetSpec, memberStatus *util.MongoStatus) error {
	members := []util.Member{}

	for _, member := range strings.Split(mongoConfig.Members, ",") {
		tokens := strings.Split(member, ":")
		id, _ := strconv.ParseInt(tokens[0], 10, 32)
		host := fmt.Sprintf("%s:%s", tokens[2], tokens[3])
		members = append(members, util.Member{ID: int(id), Host: host})
	}
	hasStaleConfig := memberStatus.ReplSetId == "" && mongoConfig.ReplSetId != ""
	log.Println("Stale config", hasStaleConfig)
	if memberStatus.ReplSetName == "" && !hasStaleConfig {
		log.Println("Initiating replset", m.cfg.RSName, members[0:1])
		if err := m.mongoCli.ReplSetInitiate(m.cfg.RSName, members[0:1]); err != nil {
			log.Panicln("Initiating primary only member failed", err)
		}
		if len(members) > 1 {
			log.Println("waiting a while before add secondaries")
			time.Sleep(3 * time.Second)
			log.Println("Adding secondaries")
			if err := m.mongoCli.AddMember(members[1:]); err != nil {
				log.Panic("Adding secondary members error", err)
			}
		}
	} else {
		currentMembers := m.ParseMembers(memberStatus.Members)
		desiredMember := m.ParseMembers(mongoConfig.Members)
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
			cfg, err := m.mongoCli.ReplSetGetConfig()
			if err != nil {
				log.Println("couldnt get replconfig while reconfiguring", err)
				return err
			}
			newMembers := []util.Member{}
			for _, m := range desiredMember {
				newMembers = append(newMembers, m)
			}
			if hasStaleConfig {
				cfg = &util.ReplSetConfig{
					Config: util.Replset{
						ID: m.cfg.RSName,
					},
				}
			}
			cfg.Config.Members = newMembers
			if err := m.mongoCli.ReplSetReconfig(&cfg.Config); err != nil {
				log.Println("Reconfiguring replset error with", newMembers, err)
				return err
			}
		}

		log.Println("Checking reconfiguration", currentMembers, desiredMember)
	}
	return nil
}
func (w *MongoController) ParseMembers(membersStr string) map[int]util.Member {
	members := make(map[int]util.Member)
	for _, m := range strings.Split(membersStr, ",") {
		tokens := strings.Split(m, ":")
		if len(tokens) < 4 {
			continue
		}
		id, _ := strconv.ParseInt(tokens[0], 10, 32)
		members[int(id)] = util.Member{ID: int(id), Host: fmt.Sprintf("%s:%s", tokens[2], tokens[3])}
	}
	return members
}
func (m *MongoController) wipeDB() error {

	log.Println("wiping dbpath", m.cfg.DBPath)
	info, err := os.Stat(m.cfg.DBPath)
	isExists := err != nil || info.IsDir()
	if !isExists {
		log.Println("Directory not exists")
		return nil
	}
	entries, err := os.ReadDir(m.cfg.DBPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryPath := filepath.Join(m.cfg.DBPath, entry.Name())
		err := os.RemoveAll(entryPath)
		if err != nil {
			log.Println("error while removing", entryPath, err)
			return err
		}
	}
	return nil
}
func (m *MongoController) checkWipeRequirment(replConfig MongoReplSetSpec, memberCfg *util.MongoStatus) bool {
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

func (m *MongoController) GetMemberStatus() (*util.MongoStatus, error) {
	mongoStatus := &util.MongoStatus{
		NodeId:    m.cfg.NodeID,
		NodeName:  m.cfg.NodeName,
		MongoAddr: m.cfg.Addr,
		MongoPort: m.cfg.Port,
	}
	status, err := m.mongoCli.ReplSetGetStatus()
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
	config, err := m.mongoCli.ReplSetGetConfig()
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
