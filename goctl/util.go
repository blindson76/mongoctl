package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/reactivex/rxgo/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongodMgm struct {
	DBPath   string
	BindIp   string
	BindPort string
	ReplSet  string
	Auth     bool
	KeyFile  string
	proc     *os.Process
	OnExit   func(state *os.ProcessState)
	exitCh   chan *os.ProcessState
}

func (m *MongodMgm) Start(extArgs ...string) error {
	m.exitCh = make(chan *os.ProcessState)
	args := []string{"--bind_ip", m.BindIp, "--port", m.BindPort, "--dbpath", m.DBPath}
	if m.Auth {
		args = append(args, "--auth", "--keyFile", m.KeyFile)

	}
	if m.ReplSet != "" {
		args = append(args, "--replSet", m.ReplSet)
	}
	args = append(args, extArgs...)
	log.Println("Starting mongod process with:", strings.Join(args, " "))
	cmd := exec.Command("mongod", args...)
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stdout
	err := cmd.Start()
	if err != nil {
		return err
	}
	m.proc = cmd.Process
	log.Println("proc started", m.DBPath)
	if m.OnExit != nil {
		go func() {
			exit, _ := m.proc.Wait()
			m.OnExit(exit)
		}()
	}
	go func() {
		ps, _ := m.proc.Wait()
		m.exitCh <- ps
	}()
	return nil
}
func (m *MongodMgm) Client() *MongoClient {
	return &MongoClient{
		serverIp:   m.BindIp,
		serverPort: m.BindPort,
		replSet:    m.ReplSet,
	}
}
func (m *MongodMgm) WithOptions() *MongoClient {
	return &MongoClient{
		serverIp:   m.BindIp,
		serverPort: m.BindPort,
		replSet:    m.ReplSet,
	}
}
func (m *MongodMgm) Kill() error {
	return m.proc.Kill()
}

func (m *MongodMgm) ShutdownWithTimeout(d time.Duration) *os.ProcessState {
	ch := make(chan error)
	if net.ParseIP(m.BindIp).IsLoopback() || m.Auth {
		log.Println("sending shotdown command to mongodb")
		cli := m.Client()
		err := cli.ConnectWithOptions(func(opts *options.ClientOptions) *options.ClientOptions {
			return opts.SetDirect(true)
		})
		if err != nil {
			log.Println("mongo connect error for shutdown command")
		} else {
			cli.cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "shutdown", Value: 1}}).Err()
		}

	} else {
		log.Println("sending INT signal to mongod process")
		m.proc.Signal(os.Interrupt)
	}
	killed := false
	timer := time.AfterFunc(d, func() {
		killed = true
		ch <- m.proc.Kill()

	})
	pstate := <-m.exitCh
	if !killed {
		timer.Stop()
	}
	return pstate
}

type MongoClient struct {
	serverIp   string
	serverPort string
	userName   string
	password   string
	replSet    string
	cli        *mongo.Client
}

func (c *MongoClient) Connect() error {
	connectOpts := options.Client().SetHosts([]string{fmt.Sprintf("%s:%s", c.serverIp, c.serverPort)})
	if c.userName != "" && c.password != "" {
		connectOpts.SetAuth(options.Credential{
			Username: c.userName,
			Password: c.password,
		})
	}
	if c.replSet != "" {
		connectOpts.SetReplicaSet(c.replSet)
	}
	client, err := mongo.Connect(connectOpts)
	c.cli = client
	return err
}
func (c *MongoClient) ConnectWithOptions(optFn func(opts *options.ClientOptions) *options.ClientOptions) error {
	connectOpts := options.Client().SetHosts([]string{fmt.Sprintf("%s:%s", c.serverIp, c.serverPort)})
	if c.userName != "" && c.password != "" {
		connectOpts.SetAuth(options.Credential{
			Username: c.userName,
			Password: c.password,
		})
	}
	if c.replSet != "" {
		connectOpts.SetReplicaSet(c.replSet)
	}
	client, err := mongo.Connect(optFn(connectOpts))
	c.cli = client
	return err
}

func (c *MongoClient) ReplSetGetStatus() (*ReplSetStatus, error) {
	var activeReplStatus ReplSetStatus
	err := c.cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&activeReplStatus)
	if err == nil {
		return &activeReplStatus, nil

	}
	log.Println("ReplSetGetStatus err", err)
	switch e := err.(type) {
	case mongo.CommandError:
		if e.Name == "NoReplicationEnabled" {
			return nil, nil
		} else if e.Name == "NotYetInitialized" {
			return nil, nil
		}
	}
	return nil, err
}
func (c *MongoClient) ReplSetGetConfig() (*ReplSetConfig, error) {
	var activeReplCfg ReplSetConfig
	err := c.cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&activeReplCfg)
	if err == nil {
		return &activeReplCfg, nil

	}
	log.Println("ReplSetGetStatus err", err)
	switch e := err.(type) {
	case mongo.CommandError:
		if e.Name == "NoReplicationEnabled" {
			return nil, nil
		} else if e.Name == "NotYetInitialized" {
			return nil, nil
		}
	}
	return nil, err
}
func (c *MongoClient) ReplSetGetConfigOffline() (*ReplSetConfig, error) {
	var activeReplCfg ReplSetConfig
	err := c.cli.Database("local").Collection("system.replset").FindOne(ctx, bson.D{{Key: "_id", Value: MONGO_RSNAME}}).Decode(&activeReplCfg.Config)
	if err != nil {
		log.Println("not found")
	}
	return &activeReplCfg, nil
}
func (c *MongoClient) ReplSetReconfig(replSetCfg *Replset) error {
	replSetCfg.Version += 1
	cmd := bson.D{
		{Key: "replSetReconfig", Value: replSetCfg},
		{Key: "force", Value: true},
	}
	log.Println("reconf cmd", cmd)

	return c.cli.Database("admin").RunCommand(context.TODO(), cmd).Err()
}
func (c *MongoClient) SetAuthentication(user, pass string) *MongoClient {
	c.userName = user
	c.password = pass
	return c
}
func (c *MongoClient) SetReplication(replSet string) *MongoClient {
	c.replSet = replSet
	return c
}
func (c *MongoClient) AddMember(members []Member) error {
	activeCfg, err := c.ReplSetGetConfig()
	if err != nil {
		return err
	}
	activeCfg.Config.Members = append(activeCfg.Config.Members, members...)
	return c.ReplSetReconfig(&activeCfg.Config)
}

func (c *MongoClient) GetOplogWindow() ([]*OpLog, error) {
	var oplogFirst OpLog
	var oplogLast OpLog
	opts := options.FindOne().SetSort(bson.D{{Key: "$natural", Value: -1}})
	err := c.cli.Database("local").Collection("oplog.rs").FindOne(context.TODO(), bson.D{}, opts).Decode(&oplogLast)
	if err != nil {
		return nil, err
	}
	opts = options.FindOne().SetSort(bson.D{{Key: "$natural", Value: 1}})
	err = c.cli.Database("local").Collection("oplog.rs").FindOne(context.TODO(), bson.D{}, opts).Decode(&oplogFirst)
	if err != nil {
		return nil, err
	}
	return []*OpLog{&oplogFirst, &oplogLast}, nil
}

func (c *MongoClient) ReplSetInitiate(rsName string, members []Member) error {
	log.Println(rsName, members)
	initCmd := bson.D{{Key: "replSetInitiate", Value: bson.D{
		{Key: "_id", Value: rsName},
		{Key: "members", Value: members},
	}}}
	return c.cli.Database("admin").RunCommand(context.TODO(), initCmd).Err()
}
func (c *MongoClient) HasUser(user string) (bool, error) {

	var users bson.M
	err := c.cli.Database("admin").RunCommand(ctx, bson.D{{Key: "usersInfo", Value: user}}).Decode(&users)
	if err != nil {
		return false, err
	}
	return (users["users"] != nil && len(users["users"].(bson.A)) > 0), nil
}

func (c *MongoClient) CreateUser(user, pass string) error {

	createCmd := bson.D{
		{Key: "createUser", Value: user},
		{Key: "pwd", Value: pass},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}},
		}},
	}
	return c.cli.Database("admin").RunCommand(ctx, createCmd).Err()
}

func ConsulWatchKey[T any](path string) rxgo.Observable {

	var lastIndex uint64 = 0
	waitTime := 5 * time.Second
	opts := &capi.QueryOptions{
		WaitIndex: lastIndex,
		WaitTime:  waitTime}
	opts = opts.WithContext(ctx)
	items := make(chan rxgo.Item)
	go func() {
		defer close(items)
		for {
			opts.WaitIndex = lastIndex
			value, meta, err := kv.Get(path, opts)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Println("stopping watch")
					return
				}
				time.Sleep(time.Second * 2)
				continue
			}

			if meta != nil {
				if lastIndex == meta.LastIndex {
					continue
				}
				lastIndex = meta.LastIndex
			}
			var obj T
			err = json.Unmarshal(value.Value, &obj)
			if err != nil {
				items <- rxgo.Of(nil)
			} else {
				items <- rxgo.Of(obj)
			}
		}
	}()
	return rxgo.FromChannel(items)
}

func ConsulWatcKeyList[T any](path string) rxgo.Observable {

	var lastIndex uint64 = 0
	waitTime := 5 * time.Second
	opts := &capi.QueryOptions{
		WaitIndex: lastIndex,
		WaitTime:  waitTime}
	opts = opts.WithContext(ctx)
	items := make(chan rxgo.Item)
	go func() {
		defer close(items)
		for {
			opts.WaitIndex = lastIndex
			value, meta, err := kv.List(path, opts)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Println("stopping watch")
					return
				}
				time.Sleep(time.Second * 2)
				continue
			}

			if meta != nil {
				if lastIndex == meta.LastIndex {
					continue
				}
				lastIndex = meta.LastIndex
			}

			elems := make([]T, len(value))
			for i, p := range value {
				var val T
				err = json.Unmarshal(p.Value, &val)
				if err != nil {
					log.Println("unmarsh err", err)
				} else {
					elems[i] = val
				}
			}
			items <- rxgo.Of(elems)
		}
	}()
	return rxgo.FromChannel(items)
}
