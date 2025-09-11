package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/hashicorp/consul/api"
)

type KafkaStatus struct {
	Name           string `json:"name"`
	BrokerAddr     string `json:"brokerAddr"`     // e.g. "10.10.51.1:9092"
	ControllerAddr string `json:"controllerAddr"` // e.g. "10.10.51.1:9093"
	ID             string `json:"id"`             // node.id
	StorageID      string `json:"storageId"`      // (dinamik quorum initial-controllers için gerekirdi; artık kullanmıyoruz)
}

type ServerProperties struct {
	ID               string
	ControllerAddr   string // "ip:9093"
	BrokerAddr       string // "ip:9092"
	MetaLogDir       string // C:/... (forward slash)
	LogDir           string // C:/...
	BootstrapServers string // "ip1:9093,ip2:9093,ip3:9093"
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

func kafkaPrestartJob() {
	log.Println("kafka prestart")

	// 1) Mevcut düğüm durumunu Consul KV'ye yaz
	self := &KafkaStatus{
		Name:           os.Getenv("NODE_NAME"),
		BrokerAddr:     fmt.Sprintf("%s:9092", os.Getenv("MONGO_ADDR")),
		ControllerAddr: fmt.Sprintf("%s:9093", os.Getenv("MONGO_ADDR")),
		ID:             os.Getenv("NODE_ID"),
		StorageID:      os.Getenv("KAFKA_STORAGE_ID"), // kullanılmıyor ama saklı kalsın
	}
	val, _ := json.Marshal(self)

	_, err := kv.Put(&api.KVPair{
		Key:   fmt.Sprintf("status/kafka/%s", self.Name),
		Value: val,
	}, nil)
	if err != nil {
		log.Fatal(err)
	}

	// 2) En az 3 controller adresi toplayana kadar bekle
	var entries []KafkaStatus
	for items := range ConsulWatcKeyList[KafkaStatus]("status/kafka").Observe() {
		entries = items.V.([]KafkaStatus)
		if len(entries) >= 3 {
			break
		}
	}

	// 3) Bootstrap stringleri oluştur
	var ctrlEndpoints []string
	for _, e := range entries {
		ctrlEndpoints = append(ctrlEndpoints, e.ControllerAddr) // "ip:9093"
	}
	bootstrap := strings.Join(ctrlEndpoints, ",")

	// 4) Konfig dosyasını yaz
	alloc := normalize(os.Getenv("NOMAD_ALLOC_DIR"))
	cfgFile := normalize(filepath.Join(alloc, "server.properties"))

	props := &ServerProperties{
		ID:               self.ID,
		ControllerAddr:   self.ControllerAddr,
		BrokerAddr:       self.BrokerAddr,
		MetaLogDir:       normalize(os.Getenv("KAFKA_META_DIR")),
		LogDir:           normalize(os.Getenv("KAFKA_DATA_DIR")),
		BootstrapServers: bootstrap,
	}

	mode := "combined"
	//mode := strings.ToLower(strings.TrimSpace(os.Getenv("KAFKA_NODE_MODE"))) // "controller" | "combined"
	var tplStr string
	switch mode {
	case "combined":
		tplStr = combinedTpl
	default:
		tplStr = controllerTpl
	}
	tpl := template.Must(template.New("kafka").Parse(tplStr))

	if err := os.MkdirAll(filepath.Dir(cfgFile), 0755); err != nil {
		log.Fatal(err)
	}
	fs, err := os.OpenFile(cfgFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("couldn't open file for write: %v", err)
	}
	if err := tpl.Execute(fs, props); err != nil {
		log.Fatalf("template error: %v", err)
	}
	_ = fs.Close()
	log.Println("Wrote configuration:", cfgFile)

	// 5) storage format (dinamik quorum)
	formatMode := strings.ToLower(strings.TrimSpace(os.Getenv("KAFKA_FORMAT_MODE"))) // "standalone" | "no-initial-controllers"
	if formatMode != "standalone" && formatMode != "no-initial-controllers" {
		// varsayılan: observer olarak katıl
		formatMode = "no-initial-controllers"
	}
	args := []string{"/c", "kafka-storage.bat", "format",
		"--cluster-id", os.Getenv("KAFKA_CLUSTER_ID"),
		"--config", cfgFile,
		"--ignore-formatted",
	}
	if formatMode == "standalone" {
		args = append(args, "--standalone")
	} else {
		args = append(args, "--no-initial-controllers")
	}
	runCmd("cmd", args...)
}

func kafkaServerJob() {
	log.Println("Starting kafka-server")

	cfgFile := normalize(filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "server.properties"))
	env := baseEnv()

	// LOG_DIR (runtime log dosyaları için)
	env = append(env, fmt.Sprintf("LOG_DIR=%s", normalize(os.Getenv("NOMAD_ALLOC_DIR"))))

	cmd := exec.Command("cmd", "/c", "kafka-server-start.bat", cfgFile)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
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

	err := cmd.Wait()
	log.Println("Kafka exited:", err)
}

/* ---------------- helpers ---------------- */

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

func runCmd(name string, args ...string) {
	log.Println("RUN:", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Env = baseEnv()
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		sc := bufio.NewScanner(strings.NewReader(string(out)))
		for sc.Scan() {
			log.Println(sc.Text())
		}
	}
	if err != nil {
		log.Fatalf("command error: %v", err)
	}
}
