package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

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
)
var (
	cli   *capi.Client
	sess  *capi.Session
	kv    *capi.KV
	agent *capi.Agent
)
var (
	KV_PATH string
)
var (
	ctx context.Context
)

func main() {
	configure()
	ctx, _ = context.WithTimeout(context.Background(), 10*time.Second)
	var err error
	conf := capi.DefaultConfig()
	conf.Address = CONSUL_HTTP_ADDR
	cli, err = capi.NewClient(conf)
	if err != nil {
		panic(err)
	}
	sess = cli.Session()
	kv = cli.KV()
	agent = cli.Agent()

	log.Println("start")
	prestart := false
	controller := false
	test := false
	mongo := false
	flag.BoolVar(&prestart, "prestart", false, "")
	flag.BoolVar(&controller, "controller", false, "")
	flag.BoolVar(&mongo, "mongo", false, "")
	flag.BoolVar(&test, "test", false, "")
	flag.Parse()

	if prestart {
		log.Println("start prestart task")
		prestart_job()
	} else if controller {
		controllerJob()
	} else if mongo {
		mongoWrapper()
	} else if test {
		testJob()
	}

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

	viper.SetDefault("nomad.addr", "http://127.0.0.1:4646")
	viper.BindEnv("nomad.addr", "NOMAD_ADDR")

	viper.SetDefault("consul.http.addr", "http://127.0.0.1:8500")
	viper.BindEnv("consul.http.addr", "CONSUL_HTTP_ADDR")

	viper.SetDefault("cluster.size", 6)
	viper.BindEnv("cluster.size", "CLUSTER_SIZE")

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

	KV_PATH = "status/mongo"

}
