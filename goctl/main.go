package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"example.com/goctl/replset"
	store "example.com/goctl/store"
	capi "github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
)

var (
	MONGO_DATA_DIR    string
	MONGO_ADDR        string
	MONGO_PORT        string
	MONGO_LOCAL_ADDR  string
	MONGO_LOCAL_PORT  string
	MONGO_SECRET_FILE string
	CSB_IP            string
	NOMAD_ADDR        string
	MONGO_RSNAME      string
	CONSUL_HTTP_ADDR  string
	NODE_ID           string
	NODE_NAME         string
	CLUSTER_SIZE      uint
	QUROUM_SIZE       uint

	KAFKA_CLUSTER_ID      string
	KAFKA_STORAGE_ID      string
	KAFKA_LOG_DIR         string
	KAFKA_DATA_DIR        string
	KAFKA_META_DIR        string
	KAFKA_BROKER_ADDR     string
	KAFKA_CONTROLLER_ADDR string
	KAFKA_BROKER_PORT     string
	KAFKA_CONTROLLER_PORT string
)
var (
	cli   *capi.Client
	sess  *capi.Session
	kv    *capi.KV
	agent *capi.Agent
)

var (
	ctx context.Context
)

func main() {
	configure()

	ms, _ := store.NewConsulStore[replset.MongoCandidateReport,
		replset.MongoReplSetSpec,
		replset.MongoHealthStatus,
	](CONSUL_HTTP_ADDR, store.ConsulStoreConfig{
		CandidateReportPath: "status/mongo",
		HealthStatusPath:    "health/mongo",
		ReplSetConfigPath:   "config/mongo",
	})
	mc := replset.NewMongoController(replset.MongoConfig{
		Addr:      MONGO_ADDR,
		Port:      MONGO_PORT,
		LocalAddr: MONGO_LOCAL_ADDR,
		LocalPort: MONGO_LOCAL_PORT,
		DBPath:    MONGO_DATA_DIR,
		RSName:    MONGO_RSNAME,
		NodeName:  NODE_NAME,
		NodeID:    NODE_ID,
		KeyFile:   MONGO_SECRET_FILE,
	}, ms)
	ks, _ := store.NewConsulStore[replset.KafkaCandidateReport,
		replset.KafkaReplSetSpec,
		replset.KafkaHealthStatus,
	](CONSUL_HTTP_ADDR, store.ConsulStoreConfig{
		CandidateReportPath: "status/kafka",
		HealthStatusPath:    "health/kafka",
		ReplSetConfigPath:   "config/kafka",
	})
	kc := replset.NewKafkaController(replset.KafkaConfig{
		ClusterID:      KAFKA_CLUSTER_ID,
		StorageID:      KAFKA_STORAGE_ID,
		NodeID:         NODE_ID,
		NodeName:       NODE_NAME,
		LogDir:         KAFKA_LOG_DIR,
		MetaDir:        KAFKA_META_DIR,
		DatDir:         KAFKA_DATA_DIR,
		BrokerAddr:     KAFKA_BROKER_ADDR,
		BrokerPort:     KAFKA_BROKER_PORT,
		ControllerAddr: KAFKA_CONTROLLER_ADDR,
		ControllerPort: KAFKA_CONTROLLER_PORT,
	}, ks)
	replType := ""
	taskType := ""
	flag.StringVar(&replType, "type", "test", "repl type: kafka|mongo")
	flag.StringVar(&taskType, "task", "test", "task type: prestart|controller|member")
	flag.Parse()
	var ctrl replset.ReplicaController

	switch replType {
	case "kafka":
		ctrl = kc
	case "mongo":
		ctrl = mc
	}
	switch taskType {
	case "prestart":
		ctrl.PreStartTask(NODE_NAME)
	case "controller":
		ctrl.ControllerTask()
	case "member":
		log.Println("Starting member task")
		ctrl.MemberTask()
	}
	log.Println("Finish")

}

func configure() {
	viper.SetDefault("mongo.address", "127.0.0.1")
	viper.BindEnv("mongo.address", "MONGO_ADDR")

	viper.SetDefault("mongo.port", "27017")
	viper.BindEnv("mongo.port", "MONGO_PORT")

	viper.SetDefault("mongo.localaddress", "127.0.0.1")
	viper.BindEnv("mongo.localaddress", "MONGO_LOCAL_ADDR")

	viper.SetDefault("mongo.localport", "27017")
	viper.BindEnv("mongo.localport", "MONGO_LOCAL_PORT")

	viper.SetDefault("mongo.dbpath", filepath.Join(os.TempDir(), "mongo"))
	viper.BindEnv("mongo.dbpath", "MONGO_DB_PATH")

	viper.SetDefault("mongo.rsname", "rs0")
	viper.BindEnv("mongo.rsname", "MONGO_RS_NAME")

	viper.SetDefault("mongo.secretfile", "")
	viper.BindEnv("mongo.secretfile", "MONGO_SECRET_FILE")

	viper.SetDefault("node.id", "0")
	viper.BindEnv("node.id", "NODE_ID")

	viper.SetDefault("node.name", "node-0")
	viper.BindEnv("node.name", "NODE_NAME")

	viper.SetDefault("nomad.addr", "http://127.0.0.1:14646")
	viper.BindEnv("nomad.addr", "NOMAD_ADDR")

	viper.SetDefault("consul.http.addr", "http://127.0.0.1:8500")
	viper.BindEnv("consul.http.addr", "CONSUL_HTTP_ADDR")

	viper.SetDefault("cluster.size", 6)
	viper.BindEnv("cluster.size", "CLUSTER_SIZE")

	//kafka
	viper.SetDefault("kafka.storageid", "")
	viper.BindEnv("kafka.storageid", "KAFKA_STORAGE_ID")
	viper.SetDefault("kafka.clusterid", "")
	viper.BindEnv("kafka.clusterid", "KAFKA_CLUSTER_ID")
	viper.SetDefault("kafka.datadir", "")
	viper.BindEnv("kafka.datadir", "KAFKA_DATA_DIR")
	viper.SetDefault("kafka.logdir", "")
	viper.BindEnv("kafka.logdir", "KAFKA_LOG_DIR")
	viper.SetDefault("kafka.metadir", "")
	viper.BindEnv("kafka.metadir", "KAFKA_META_DIR")
	viper.SetDefault("kafka.broker.addr", "")
	viper.BindEnv("kafka.broker.addr", "KAFKA_BROKER_ADDR")
	viper.SetDefault("kafka.broker.port", "")
	viper.BindEnv("kafka.broker.port", "KAFKA_BROKER_PORT")
	viper.SetDefault("kafka.controller.addr", "")
	viper.BindEnv("kafka.controller.addr", "KAFKA_CONTROLLER_ADDR")
	viper.SetDefault("kafka.controller.port", "")
	viper.BindEnv("kafka.controller.port", "KAFKA_CONTROLLER_PORT")

	KAFKA_CLUSTER_ID = viper.GetString("kafka.clusterid")
	KAFKA_STORAGE_ID = viper.GetString("kafka.storageid")
	KAFKA_DATA_DIR = viper.GetString("kafka.datadir")
	KAFKA_LOG_DIR = viper.GetString("kafka.logdir")
	KAFKA_META_DIR = viper.GetString("kafka.metadir")
	KAFKA_BROKER_ADDR = viper.GetString("kafka.broker.addr")
	KAFKA_BROKER_PORT = viper.GetString("kafka.broker.port")
	KAFKA_CONTROLLER_ADDR = viper.GetString("kafka.controller.addr")
	KAFKA_CONTROLLER_PORT = viper.GetString("kafka.controller.port")

	NODE_ID = viper.GetString("node.id")
	NODE_NAME = viper.GetString("node.name")
	MONGO_RSNAME = viper.GetString("mongo.rsname")
	MONGO_DATA_DIR = viper.GetString("mongo.dbpath")
	MONGO_SECRET_FILE = viper.GetString("mongo.secretfile")

	MONGO_ADDR = viper.GetString("mongo.address")
	MONGO_PORT = viper.GetString("mongo.port")

	MONGO_LOCAL_ADDR = viper.GetString("mongo.localaddress")
	MONGO_LOCAL_PORT = viper.GetString("mongo.localport")

	NOMAD_ADDR = viper.GetString("nomad.addr")
	CONSUL_HTTP_ADDR = viper.GetString("consul.http.addr")

	CLUSTER_SIZE = viper.GetUint("cluster.size")
	QUROUM_SIZE = uint(CLUSTER_SIZE/2) + 1
	conf, _ := json.MarshalIndent(viper.AllSettings(), " ", " ")
	log.Println(string(conf))

}
