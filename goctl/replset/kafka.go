package replset

import (
	"example.com/goctl/store"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	Members string
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
	//TODO implement me
	return true
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
	replicaSetController[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]
	cfg KafkaConfig
}

func NewKafkaController(cfg KafkaConfig, str store.Provider[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]) *KafkaController {
	mc := &KafkaController{
		cfg: cfg,
	}
	mc.replicaSetController = replicaSetController[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]{
		collector: mc,
		store:     str,
	}
	return mc
}
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
	finfo, err := os.Stat(k.cfg.DatDir)
	if err == nil {
		report.MTime = finfo.ModTime().String()
	}
	return report, nil

}

func (k KafkaController) generateReplConfig(cs []KafkaCandidateReport) *KafkaReplSetSpec {
	if len(cs) == 0 {
		return &KafkaReplSetSpec{Members: ""}
	}
	// Sort newest first (parse MTime as int64 to avoid lexicographic pitfalls)
	sort.Slice(cs, func(i, j int) bool {
		ti, _ := strconv.ParseInt(cs[i].MTime, 10, 64)
		tj, _ := strconv.ParseInt(cs[j].MTime, 10, 64)
		return ti > tj
	})
	n := 3
	if len(cs) < n {
		n = len(cs)
	}
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = cs[i].NodeID
	}
	return &KafkaReplSetSpec{Members: strings.Join(ids, ",")}
}

func (k KafkaController) memberTask(s <-chan KafkaReplSetSpec) <-chan KafkaHealthStatus {
	out := make(chan KafkaHealthStatus, 1)
	go func() {
		defer close(out)
		for range s {
			out <- KafkaHealthStatus{
				NodeName: k.cfg.NodeName,
				NodeId:   k.cfg.NodeID,
				Status:   "healthy",
			}
		}
	}()
	return out
}

var (
	// Controller-only (dinamik quorum)
	controllerTpl = strings.TrimSpace(`
node.id={{.ID}}
process.roles=controller

listeners=CONTROLLER://{{.ControllerAddr}}
advertised.listeners=CONTROLLER://{{.ControllerAddr}}
controller.listener.names=CONTROLLER
listener.security.protocol.map=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT

controller.quorum.bootstrap.servers={{.BootstrapServers}}

metadata.log.dir={{.MetaLogDir}}
`)

	// Combined mode (broker + controller)
	combinedTpl = strings.TrimSpace(`
node.id={{.ID}}
process.roles=broker,controller

# broker listener (clients buraya bağlanır)
listeners=PLAINTEXT://{{.BrokerAddr}},CONTROLLER://{{.ControllerAddr}}
advertised.listeners=PLAINTEXT://{{.BrokerAddr}}
inter.broker.listener.name=PLAINTEXT
controller.listener.names=CONTROLLER
listener.security.protocol.map=PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT

controller.quorum.bootstrap.servers={{.BootstrapServers}}

log.dirs={{.LogDir}}
metadata.log.dir={{.MetaLogDir}}
num.partitions=3
`)
)

func normalize(p string) string {
	// Windows'ta forward slash tercih ediyoruz (Kafka conf için güvenli)
	clean := filepath.Clean(p)
	return strings.ReplaceAll(clean, `\`, `/`)
}
