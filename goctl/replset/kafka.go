package replset

import (
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"example.com/goctl/store"
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
	cfg KafkaConfig
}

func NewKafkaController(cfg KafkaConfig, str store.Provider[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]) *KafkaController {
	mc := &KafkaController{
		cfg: cfg,
	}
	mc.replicaSetControl = replicaSetControl[KafkaCandidateReport, KafkaReplSetSpec, KafkaHealthStatus]{
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
	bootstrapServers := []string{}
	bootstrapServersStorage := []string{}
	for _, m := range cs {
		bootstrapServers = append(bootstrapServers, fmt.Sprintf("%s:%s", m.ControllerAddr, m.ControllerPort))
		bootstrapServersStorage = append(bootstrapServersStorage, fmt.Sprintf("%s@%s:%s:%s", m.NodeID, m.ControllerAddr, m.ControllerPort, m.StorageID))
	}
	return &KafkaReplSetSpec{BootstrapServers: strings.Join(bootstrapServers, ","), BootstrapServersStorage: strings.Join(bootstrapServersStorage, ",")}
}

func (k KafkaController) memberTask(s <-chan KafkaReplSetSpec) <-chan KafkaHealthStatus {
	log.Println("Member task started")
	out := make(chan KafkaHealthStatus, 1)
	defer close(out)
	for spec := range s {
		log.Println("Read new replset config", spec)
		cfgFile, err := k.createConfigFile(spec)
		if err != nil {
			log.Println("create config file error:", err)
			continue
		}
		if err := k.formatStorage(spec); err != nil {
			log.Println("Format storage error", err)
			continue
		}

		if err := k.startServer(cfgFile); err != nil {
			log.Println("Server start error", err)
			continue
		}

		out <- KafkaHealthStatus{
			NodeName: k.cfg.NodeName,
			NodeId:   k.cfg.NodeID,
			Status:   "healthy",
		}
	}
	return out
}
func (k KafkaController) formatStorage(s KafkaReplSetSpec) error {
	cfgFile := normalize(filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "server.properties"))
	env := baseEnv()
	env = append(env, fmt.Sprintf("LOG_DIR=%s", normalize(filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "kafka-logs"))))

	cmd := exec.Command("cmd", "/c", "kafka-storage.bat", "format", "-t", s.ClusterID, "-c", cfgFile, "--initial-controllers", s.BootstrapServersStorage)
	cmd.Env = env

	if out, err := cmd.CombinedOutput(); err != nil {
		return err
	} else {
		log.Println("storage out:", string(out))
	}
	return nil

}
func (k KafkaController) createConfigFile(s KafkaReplSetSpec) (string, error) {
	alloc := normalize(os.Getenv("NOMAD_ALLOC_DIR"))
	cfgFile := normalize(filepath.Join(alloc, "server.properties"))

	props := &ServerProperties{
		ID:               k.cfg.NodeID,
		ControllerAddr:   fmt.Sprintf("%s:%s", k.cfg.ControllerAddr, k.cfg.ControllerPort),
		BrokerAddr:       fmt.Sprintf("%s:%s", k.cfg.BrokerAddr, k.cfg.BrokerPort),
		MetaLogDir:       normalize(k.cfg.MetaDir),
		LogDir:           normalize(k.cfg.DatDir),
		BootstrapServers: s.BootstrapServers,
	}

	tplStr := combinedTpl
	tpl := template.Must(template.New("kafka").Parse(tplStr))

	if err := os.MkdirAll(filepath.Dir(cfgFile), 0755); err != nil {
		return "", err
	}
	log.Println("Creating cfg file:", cfgFile)
	fs, err := os.OpenFile(cfgFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	if err := tpl.Execute(fs, props); err != nil {
		return "", err
	}
	_ = fs.Close()
	log.Println("Wrote configuration:", cfgFile)
	return cfgFile, nil
}
func (k KafkaController) startServer(cfgFile string) error {
	env := baseEnv()
	env = append(env, fmt.Sprintf("LOG_DIR=%s", normalize(filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "kafka-logs"))))
	cmd := exec.Command("cmd", "/c", "kafka-server-start.bat", cfgFile)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	// graceful shutdown
	sigs := make(chan os.Signal, 1)
	if runtime.GOOS == "windows" {
		signal.Notify(sigs, os.Interrupt)
	} else {
		signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	}

	go func() {
		sig := <-sigs
		log.Println("Signal:", sig)
		_ = cmd.Process.Signal(os.Interrupt)
		time.Sleep(5 * time.Second)
		_ = cmd.Process.Kill()
	}()

	return nil

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
func baseEnv() []string {
	return append(os.Environ(),
		fmt.Sprintf("PATH=%s", strings.Join([]string{
			os.Getenv("PATH"),
			"D:\\kafka\\bin\\windows",
			"C:\\Users\\ubozkurt\\Downloads\\jdk-24_windows-x64_bin\\jdk-24.0.1\\bin",
		}, ";")),
		"CLASSPATH=",
	)
}
