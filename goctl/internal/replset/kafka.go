package replset

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"example.com/goctl/internal/store"
)

type KafkaCandidateReport struct {
	ControllerAddr string
	ControllerPort string
	BrokerAddr     string
	BrokerPort     string
	StorageID      string
	MTime          string
	NodeID         string
	NodeName       string
}

func (k KafkaCandidateReport) Less(other KafkaCandidateReport) bool {
	return k.MTime < other.MTime
}

func (k KafkaCandidateReport) GetId() string {
	return k.NodeID
}

type KafkaReplSetSpec struct {
	BootstrapServers        string
	BootstrapServersStorage string
	ClusterID               string
	Members                 []string
}

func (k KafkaReplSetSpec) GetMembers() []string {
	return k.Members
}

type KafkaHealthStatus struct {
	NodeName string
	NodeId   string
	Status   string
}

func (k KafkaHealthStatus) Less(other KafkaHealthStatus) bool {
	//TODO implement me
	return k.NodeName < other.NodeName
}

func (k KafkaHealthStatus) GetId() string {
	//TODO implement me
	return k.NodeName
}

func (k KafkaHealthStatus) IsHealthy() bool {
	return strings.ToUpper(k.Status) == "OK"
}

type ServerProperties struct {
	ID               string
	ControllerAddr   string // "ip:9093"
	BrokerAddr       string // "ip:9092"
	MetaLogDir       string // C:/... (forward slash)
	LogDir           string // C:/...
	BootstrapServers string // "ip1:9093,ip2:9093,ip3:9093"
}
type KafkaConfig struct {
	ClusterID      string
	StorageID      string
	NodeID         string
	NodeName       string
	LogDir         string
	MetaDir        string
	DatDir         string
	ControllerAddr string
	ControllerPort string
	BrokerAddr     string
	BrokerPort     string
}
type KafkaController struct {
	replicaSetControl[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]
	cfg        KafkaConfig
	proc       *os.Process
	state      KafkaState
	healthChan chan KafkaHealthStatus
}

func NewKafkaController(cfg KafkaConfig, nomadAddr, jobFile string, str store.Provider[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]) *KafkaController {
	mc := &KafkaController{
		cfg: cfg,
	}
	mc.replicaSetControl = replicaSetControl[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]{
		collector: mc,
		name:      "kafka",
		store:     str,
		jobFile:   jobFile,
		nomadAddr: nomadAddr,
	}
	return mc
}

type KafkaState int

const (
	KafkaInitial KafkaState = iota
	KafkaStartup
	KafkaFailed
)

func (k KafkaController) collect() (*KafkaCandidateReport, error) {
	report := &KafkaCandidateReport{
		ControllerAddr: k.cfg.BrokerAddr,
		ControllerPort: k.cfg.ControllerPort,
		BrokerAddr:     k.cfg.BrokerAddr,
		BrokerPort:     k.cfg.BrokerPort,
		StorageID:      k.cfg.StorageID,
		NodeID:         k.cfg.NodeID,
		NodeName:       k.cfg.NodeName,
	}
	fInfo, err := os.Stat(k.cfg.DatDir)
	if err == nil {
		report.MTime = fInfo.ModTime().String()
	}
	return report, nil

}

func (k KafkaController) generateReplConfig(cs []KafkaCandidateReport) KafkaReplSetSpec {
	var bootstrapServers []string
	var bootstrapServersStorage []string
	var members []string
	for _, m := range cs {
		bootstrapServers = append(bootstrapServers, fmt.Sprintf("%s:%s", m.ControllerAddr, m.ControllerPort))
		bootstrapServersStorage = append(bootstrapServersStorage, fmt.Sprintf("%s@%s:%s:%s", m.NodeID, m.ControllerAddr, m.ControllerPort, m.StorageID))
		members = append(members, m.NodeName)
	}
	return KafkaReplSetSpec{
		BootstrapServers:        strings.Join(bootstrapServers, ","),
		BootstrapServersStorage: strings.Join(bootstrapServersStorage, ","),
		ClusterID:               k.cfg.ClusterID,
		Members:                 members,
	}
}

func (k KafkaController) memberTask(s <-chan KafkaReplSetSpec) <-chan KafkaHealthStatus {
	log.Println("Member task started", "pid:", os.Getpid())
	k.healthChan = make(chan KafkaHealthStatus, 1)
	var exitChan chan *os.ProcessState
	sm := NewKafkaSM(context.Background(), k)
	go func() {
		defer close(k.healthChan)
		log.Println("waiting event")
		spec := <-s
		log.Println("Read new replset config", spec)
		inReplset := slices.Contains(spec.Members, k.cfg.NodeName)
		if inReplset {
			sm.FireStart(spec)
		}
		for {
			select {
			case exitState := <-exitChan:
				log.Println("Kafka exited", exitState)
				sm.FireExit(exitState)
				break
			}

		}
		log.Println("exiting member task")
	}()
	return k.healthChan
}
