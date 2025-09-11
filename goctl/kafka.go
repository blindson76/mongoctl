package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/hashicorp/consul/api"
)

type KafkaStatus struct {
	Name           string `json:"name"`
	BrokerAddr     string `json:"brokerAddr"`
	ControllerAddr string `json:"controllerAddr"`
	ID             string `json:"id"`
	StorageID      string `json:"storageId"`
}
type ServerProperties struct {
	ID               string
	ControllerAddr   string
	BrokerAddr       string
	MetaLogDir       string
	LogDir           string
	BootstrapServers string
}

var (
	propTpl = `
node.id={{.ID}}
process.roles=broker,controller

listeners=PLAINTEXT://{{.BrokerAddr}},CONTROLLER://{{.ControllerAddr}}
advertise.listeners=PLAINTEXT://{{.BrokerAddr}},CONTROLLER://{{.ControllerAddr}}

controller.quorum.bootstrap.servers={{.BootstrapServers}}

controller.listener.names=CONTROLLER

inter.broker.listener.name=PLAINTEXT

log.dirs={{.LogDir}}
metadata.log.dir={{.MetaLogDir}}
num.partitions=3
	`
)

func kafkaPrestartJob() {
	log.Println("kafka prestart")
	value, err := json.Marshal(&KafkaStatus{
		Name:           NODE_NAME,
		BrokerAddr:     fmt.Sprintf("%s:9092", MONGO_ADDR),
		ControllerAddr: fmt.Sprintf("%s:9093", MONGO_ADDR),
		ID:             NODE_ID,
		StorageID:      os.Getenv("KAFKA_STORAGE_ID"),
	})
	if err != nil {
		panic(err)
	}
	_, err = kv.Put(&api.KVPair{
		Key:   fmt.Sprintf("status/kafka/%s", NODE_NAME),
		Value: value,
	}, nil)
	if err != nil {
		panic(err)
	}

	tpl, err := template.New("kafka").Parse(propTpl)
	cfgFile := filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "server.properties")
	cfgFile = strings.ReplaceAll(filepath.Clean(cfgFile), `\`, `/`)

	var entries []KafkaStatus
	for items := range ConsulWatcKeyList[KafkaStatus]("status/kafka").Observe() {
		log.Println("changed", items)
		entries = items.V.([]KafkaStatus)
		if len(entries) >= 3 {
			log.Println("reached required items")
			break
		}
		for _, item := range entries {
			log.Println("item", item)
		}

	}
	log.Println("configuration Done. Storage configuration")
	servers := []string{}
	bootstrapServersStr := []string{}
	for _, e := range entries {
		servers = append(servers, fmt.Sprintf("%s", e.ControllerAddr))
		bootstrapServersStr = append(bootstrapServersStr, fmt.Sprintf("%s@%s:%s", e.ID, e.ControllerAddr, e.StorageID))
	}
	serversStr := strings.Join(servers, ",")

	serverCfg := &ServerProperties{
		BrokerAddr:       fmt.Sprintf("%s:9092", MONGO_ADDR),
		ControllerAddr:   fmt.Sprintf("%s:9093", MONGO_ADDR),
		ID:               NODE_ID,
		BootstrapServers: serversStr,
		MetaLogDir:       strings.ReplaceAll(path.Clean(os.Getenv("KAFKA_META_DIR")), `\`, `/`),
		LogDir:           strings.ReplaceAll(path.Clean(os.Getenv("KAFKA_DATA_DIR")), `\`, `/`),
	}
	fs, err := os.OpenFile(cfgFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Panicln("couldn open file for write", err)
	}
	log.Println("Writing configuration to", cfgFile)
	if err := tpl.Execute(fs, serverCfg); err != nil {
		log.Println("template error", err)
	}
	log.Println("Starting kafka-storage with", bootstrapServersStr)
	args := []string{"/c", "kafka-storage.bat", "format", "-t", os.Getenv("KAFKA_CLUSTER_ID"), "-c", cfgFile, "--initial-controllers", strings.Join(bootstrapServersStr, ","), "--ignore-formatted"}
	log.Println("Starting kafka-storage with", args)
	cmd := exec.Command("cmd", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PATH=%s", strings.Join([]string{os.Getenv("PATH"),
			"D:\\kafka\\bin\\windows",
			"C:\\Users\\ubozkurt\\Downloads\\jdk-24_windows-x64_bin\\jdk-24.0.1\\bin"}, ";")),
		"CLASSPATH=",
		fmt.Sprintf("LOG_DIR=%s", strings.ReplaceAll(os.Getenv("NOMAD_ALLOC_DIR"), `\`, `/`)),
	)
	out, err := cmd.CombinedOutput()
	log.Println(string(out), err)
	//time.Sleep(10 * time.Second)

}

func kafkaServerJob() {
	log.Println("Starting kafka-server")
	cfgFile := filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "server.properties")
	cfgFile = strings.ReplaceAll(filepath.Clean(cfgFile), `\`, `/`)
	log.Println("CfgFile", cfgFile)
	cmd := exec.Command("cmd", "/c", "kafka-server-start.bat", cfgFile)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PATH=%s", strings.Join([]string{os.Getenv("PATH"),
			"D:\\kafka\\bin\\windows",
			"C:\\Users\\ubozkurt\\Downloads\\jdk-24_windows-x64_bin\\jdk-24.0.1\\bin"}, ";")),
		"CLASSPATH=",
		fmt.Sprintf("LOG_DIR=%s", strings.ReplaceAll(os.Getenv("NOMAD_ALLOC_DIR"), `\`, `/`)),
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Start()
	if err != nil {
		panic(err)
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill)
	go func() {
		for {
			log.Println("loop")
			sig := <-sigs
			log.Println("Got sign:", sig)
			cmd.Process.Signal(os.Interrupt)
			time.Sleep(5 * time.Second)
			log.Println("send kill signal")
			cmd.Process.Signal(os.Kill)
			time.Sleep(3 * time.Second)
			os.Exit(0)
		}
	}()
	err = cmd.Wait()
	log.Println("Proc exit:", err)
}
