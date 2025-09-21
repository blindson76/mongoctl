package util

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
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
			err := cli.Cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "shutdown", Value: 1}}).Err()
			if err != nil {
				return nil
			}
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

func (m *MongodMgm) Repair() error {
	cmd := exec.Command("mongod", "--dbpath", m.DBPath, "--repair")
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	log.Println("repair exit status:", cmd.ProcessState.ExitCode())
	if cmd.ProcessState.ExitCode() != 0 {
		return fmt.Errorf("mongo repair failed:%d", cmd.ProcessState.ExitCode())
	}
	return nil

}

type MongoClient struct {
	serverIp   string
	serverPort string
	userName   string
	password   string
	replSet    string
	Cli        *mongo.Client
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
	c.Cli = client
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
	c.Cli = client
	return err
}

func (c *MongoClient) ReplSetGetStatus() (*ReplSetStatus, error) {
	var activeReplStatus ReplSetStatus
	err := c.Cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&activeReplStatus)
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
	err := c.Cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&activeReplCfg)
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
func (c *MongoClient) ReplSetGetConfigOffline(rsName string) (*ReplSetConfig, error) {
	var activeReplCfg ReplSetConfig
	err := c.Cli.Database("local").Collection("system.replset").FindOne(context.TODO(), bson.D{{Key: "_id", Value: rsName}}).Decode(&activeReplCfg.Config)
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

	return c.Cli.Database("admin").RunCommand(context.TODO(), cmd).Err()
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
	err := c.Cli.Database("local").Collection("oplog.rs").FindOne(context.TODO(), bson.D{}, opts).Decode(&oplogLast)
	if err != nil {
		return nil, err
	}
	opts = options.FindOne().SetSort(bson.D{{Key: "$natural", Value: 1}})
	err = c.Cli.Database("local").Collection("oplog.rs").FindOne(context.TODO(), bson.D{}, opts).Decode(&oplogFirst)
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
	return c.Cli.Database("admin").RunCommand(context.TODO(), initCmd).Err()
}
func (c *MongoClient) HasUser(user string) (bool, error) {

	var users bson.M
	err := c.Cli.Database("admin").RunCommand(context.TODO(), bson.D{{Key: "usersInfo", Value: user}}).Decode(&users)
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
	return c.Cli.Database("admin").RunCommand(context.TODO(), createCmd).Err()
}
