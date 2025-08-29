package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Settings struct {
	ReplicaSetId bson.ObjectID `bson:"replicaSetId"`
}
type Member struct {
	Host string `bson:"host"`
}
type Replset struct {
	ID       string   `bson:"_id"`
	Members  []Member `bson:"members"`
	Term     int64    `bson:"term"`
	Settings Settings `bson:"settings"`
}

type OpLog struct {
	Ts bson.Timestamp `bson:"ts"`
}

type MongoStatusMap map[string]*MongoStatus

type MongoStatus struct {
	NodeId        string
	ReplSetId     string
	ReplSetName   string
	Members       string
	Term          int64
	OpLogFirstSec uint32
	OpLogFirstInc uint32
	OpLogLastSec  uint32
	OpLogLasttInc uint32
}

var (
	MONGO_DATA_DIR   string
	MONGO_ADDR       string
	MONGO_PORT       string
	MONGO_LOCAL_ADDR string
	MONGO_LOCAL_PORT string
	CSB_IP           string
	NOMAD_ADDR       string
	MONGO_RSNAME     string
	CONSUL_HTTP_ADDR string
	NODE_ID          string
	CLUSTER_SIZE     uint
	QUROUM_SIZE      uint
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
	log.Println("nodeid", NODE_ID, CLUSTER_SIZE, QUROUM_SIZE)
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
	flag.BoolVar(&prestart, "prestart", false, "")
	flag.BoolVar(&controller, "controller", false, "")
	flag.Parse()

	if prestart {
		log.Println("start prestart task")
		prestart_job()
	} else if controller {
		controller_job()
	}

}
func prestart_job() {
	cmd := exec.Command("mongod", "--dbpath", MONGO_DATA_DIR, "--bind_ip", MONGO_LOCAL_ADDR, "--port", MONGO_LOCAL_PORT)
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stdout
	err := cmd.Start()
	if err != nil {
		panic(err)
	}
	client, err := mongo.Connect(options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%s", MONGO_LOCAL_ADDR, MONGO_LOCAL_PORT)).SetServerSelectionTimeout(time.Second * 5))
	if err != nil {
		log.Println("mongo connect error", err)
	}
	defer func() {
		log.Println("closing db conn")
		client.Disconnect(ctx)
	}()
	var users bson.M
	err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "usersInfo", Value: "admin"}}).Decode(&users)
	userExist := (err == nil && users["users"] != nil && len(users["users"].(bson.A)) > 0)
	if userExist {
		log.Println("user already exist")
	} else {
		createCmd := bson.D{
			{Key: "createUser", Value: "admin"},
			{Key: "pwd", Value: "123"},
			{Key: "roles", Value: bson.A{
				bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}},
			}},
		}
		err = client.Database("admin").RunCommand(ctx, createCmd).Err()
		if err != nil {
			log.Panic("Failed to create user", err)
		}

	}
	var replset Replset
	var oplogFirst OpLog
	var oplogLast OpLog
	err = client.Database("local").Collection("system.replset").FindOne(ctx, bson.D{{Key: "_id", Value: MONGO_RSNAME}}).Decode(&replset)
	if err != nil {
		log.Println("not found")
	}

	opts := options.FindOne().SetSort(bson.D{{Key: "$natural", Value: -1}})
	client.Database("local").Collection("oplog.rs").FindOne(ctx, bson.D{}, opts).Decode(&oplogFirst)
	opts = options.FindOne().SetSort(bson.D{{Key: "$natural", Value: 1}})
	client.Database("local").Collection("oplog.rs").FindOne(ctx, bson.D{}, opts).Decode(&oplogLast)
	hosts := make([]string, len(replset.Members))
	for i, m := range replset.Members {
		hosts[i] = m.Host
	}

	status := &MongoStatus{
		NodeId:        NODE_ID,
		ReplSetId:     replset.Settings.ReplicaSetId.String(),
		ReplSetName:   replset.ID,
		Members:       strings.Join(hosts, ","),
		Term:          replset.Term,
		OpLogFirstSec: oplogFirst.Ts.T,
		OpLogFirstInc: oplogFirst.Ts.I,
		OpLogLastSec:  oplogLast.Ts.T,
		OpLogLasttInc: oplogLast.Ts.I,
	}
	conf := capi.DefaultConfig()
	conf.Address = CONSUL_HTTP_ADDR
	cli, err := capi.NewClient(conf)
	if err != nil {
		panic(err)
	}
	sess := cli.Session()
	kv := cli.KV()
	agent := cli.Agent()
	sessionName := fmt.Sprintf("%s.mongo-status", NODE_ID)
	var sessionId string

	//log.Println(checks)

	// sessions, _, err := sess.List(nil)
	// if err != nil {
	// 	panic(err)
	// }
	// for _, s := range sessions {
	// 	log.Println(s.ID)
	// 	sess.Destroy(s.ID, nil)
	// }
	// return

	statusVal, _, err := kv.Get(fmt.Sprintf("%s/%s", KV_PATH, NODE_ID), nil)
	if err != nil {
		panic(err)
	}
	if statusVal != nil {
		sessionId = statusVal.Session
	}
	if sessionId == "" {
		//creating session
		log.Println("creating session")
		var check string
		checks, err := agent.Checks()
		if err != nil {
			panic(err)
		}

		for k, v := range checks {
			if v.Name == "Nomad Client HTTP Check" {
				log.Println(k, v.Name)
				check = v.CheckID
				break
			}
		}
		if check == "" {
			panic("Check not found")
		}

		session, _, err := sess.Create(&capi.SessionEntry{
			Behavior:      capi.SessionBehaviorDelete,
			ServiceChecks: []capi.ServiceCheck{{ID: check}},
			Name:          sessionName,
		},
			nil)
		if err != nil {
			panic(err)
		}
		sessionId = session
	}
	log.Println("session", sessionId)
	statusStr, err := json.Marshal(status)
	if err != nil {
		panic(err)
	}
	log.Println("Mongo status:", string(statusStr))
	done, _, err := kv.Acquire(&capi.KVPair{
		Key:     fmt.Sprintf("%s/%s", KV_PATH, NODE_ID),
		Session: sessionId,
		Value:   statusStr,
	}, nil)
	log.Println("Done", done)
	//sess.Destroy(session, &capi.WriteOptions{})

	err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "shutdown", Value: 1}}).Err()
	log.Println("Shutdown:", err)
	if strings.Contains(err.Error(), "Unauthorized") {
		log.Println("send int to mongod")
		cmd.Process.Signal(os.Interrupt)
	}

	// cmd.Process.Signal(os.Interrupt)
	time.AfterFunc(time.Second*10, func() {
		log.Println("Send kill signal to mongod")
		cmd.Process.Signal(os.Kill)
	})
	procState, err := cmd.Process.Wait()
	if err != nil {
		panic(err)
	}

	log.Println("Proc exit with", procState)
	if !done {
		panic(fmt.Sprintf("Not completed: %d", procState.ExitCode()))
	}
	log.Println("end")

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

	viper.SetDefault("node.id", "node-0")
	viper.BindEnv("node.id", "NODE_ID")

	viper.SetDefault("nomad.addr", "http://127.0.0.1:4646")
	viper.BindEnv("nomad.addr", "NOMAD_ADDR")

	viper.SetDefault("consul.http.addr", "http://127.0.0.1:8500")
	viper.BindEnv("consul.http.addr", "CONSUL_HTTP_ADDR")

	viper.SetDefault("cluster.size", 6)
	viper.BindEnv("cluster.size", "CLUSTER_SIZE")

	NODE_ID = viper.GetString("node.id")
	MONGO_RSNAME = viper.GetString("mongo.rsname")
	MONGO_DATA_DIR = viper.GetString("mongo.dbpath")

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
