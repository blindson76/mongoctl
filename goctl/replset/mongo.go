package replset

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"example.com/goctl/store"
	"example.com/goctl/util"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

func NewMongoController(mongoCfg MongoConfig, nomadAddr, jobFile string, str store.Provider[MongoCandidateReport, MongoReplSetSpec, MongoHealthStatus]) *MongoController {
	mc := &MongoController{
		cfg: mongoCfg,
	}
	mc.replicaSetControl = replicaSetControl[MongoCandidateReport, MongoReplSetSpec, MongoHealthStatus]{
		collector: mc,
		store:     str,
		nomadAddr: nomadAddr,
		jobFile:   jobFile,
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
	OpLogLasttInc uint32 `json:"OpLogLastInc"`
}

func (m MongoReplSetSpec) GetMembers() []string {
	var members []string
	for _, m := range strings.Split(m.Members, ",") {
		tokens := strings.Split(m, ":")
		members = append(members, tokens[1])
	}
	return members
}

type MongoController struct {
	cfg MongoConfig
	replicaSetControl[MongoCandidateReport, MongoReplSetSpec, MongoHealthStatus]
	mongo    *util.MongodMgm
	mongoCli *util.MongoClient
}

func (m *MongoController) generateReplConfig(candidates []MongoCandidateReport) MongoReplSetSpec {
	memberStr := []string{}
	for _, m := range candidates {
		memberStr = append(memberStr, fmt.Sprintf("%s:%s:%s:%s", m.NodeId, m.NodeName, m.MongoAddr, m.MongoPort))
	}

	mongoCfg := MongoReplSetSpec{
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
func (m *MongoController) collect() (*MongoCandidateReport, error) {
	for i := 0; i < 2; i++ {
		log.Println("try attempt:", i+1)
		report, err := m.getOfflineStatus()
		if err == nil {
			return report, nil
		}
		time.Sleep(2 * time.Second)
	}

	log.Println("get offlinestatus err")

	log.Println("trying wipe db")
	if err := m.wipeDB(); err != nil {
		log.Println("wipe error", err)
	}
	return m.getOfflineStatus()
}
func (m *MongoController) getOfflineStatus() (*MongoCandidateReport, error) {

	var exitState *os.ProcessState
	mongod := util.MongodMgm{
		DBPath:   m.cfg.DBPath,
		BindIp:   m.cfg.LocalAddr,
		BindPort: m.cfg.LocalPort,
		OnExit: func(state *os.ProcessState) {
			log.Println("mongod exited", state)
			exitState = state
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
		log.Println("mongo connect error", err)
		if exitState != nil {
			log.Println("mongod exited before connect", exitState)
		}
		return nil, err
	}
	err = mongoCli.Cli.Ping(context.TODO(), nil)
	if err != nil {
		log.Println("Mongo ping error", err)
		if exitState != nil {
			log.Println("mongod exited before connect", exitState)
			if exitState.ExitCode() == 14 {

			}
		}
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
func (m *MongoController) memberTask(configChan <-chan MongoReplSetSpec) <-chan MongoHealthStatus {
	log.Println("member controller", "PID:", os.Getpid())
	out := make(chan MongoHealthStatus, 1)
	var ticker *time.Ticker
	exitChan := make(chan *os.ProcessState)
	defer func() {
		log.Println("kill mongod here")
	}()
	go func() {

		m.mongo = &util.MongodMgm{
			BindIp:   m.cfg.Addr,
			BindPort: m.cfg.Port,
			DBPath:   m.cfg.DBPath,
			ReplSet:  m.cfg.RSName,
			Auth:     true,
			KeyFile:  m.cfg.KeyFile,
			OnExit: func(state *os.ProcessState) {
				log.Println("Mongod exited", state)
				exitChan <- state
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
		} else {
			log.Println("Ping OK. Connected to mongod")
		}

		ticker = time.NewTicker(3 * time.Second)
		go func() {
			for range ticker.C {
				log.Println("Health status publishing...")
				if status, err := m.mongoCli.ReplSetGetStatus(); err == nil && status != nil {
					stat := MongoHealthStatus{
						NodeId:   m.cfg.NodeID,
						NodeName: m.cfg.NodeName,
						Status:   fmt.Sprintf("%s", status.MyState),
					}
					log.Println("Health:", stat)
					out <- stat
				}

			}
		}()
		time.Sleep(5 * time.Second)

		for {
			select {
			case exitStatus := <-exitChan:
				log.Panicln("mongod exited", exitStatus)
			case mongoConfigItem := <-configChan:

				mongoConfig := mongoConfigItem
				log.Println("fetch new mongoconfig", mongoConfigItem)
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
			}
		}

	}()
	return out

}
