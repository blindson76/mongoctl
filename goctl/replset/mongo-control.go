package replset

import (
	"example.com/goctl/util"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (m *MongoController) ParseMembers(membersStr string) map[int]util.Member {
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
