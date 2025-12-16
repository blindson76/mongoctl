package replset

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"example.com/goctl/internal/replset/kafkautil"
	"github.com/qmuntal/stateless"
	"github.com/segmentio/kafka-go"
	"golang.org/x/sys/windows"
)

type State string
type Trigger string

const (
	StateInit       State = "init"
	StateFormatting State = "formatting"
	StateStarting   State = "starting"
	StateFailed     State = "failed"

	StateStartup      State = "startup"
	StateRunning      State = "running"
	StateShuttingDown State = "shuttingDown"
)

const (
	TrigStart       Trigger = "start"
	TrigStartServer Trigger = "startServer"
	TrigDone        Trigger = "done"
	TrigReady       Trigger = "ready"
	TrigKafkaExit   Trigger = "kafkaExit"
	TrigShutdown    Trigger = "shutdown"
)

type KafkaSM struct {
	sm      *stateless.StateMachine
	ctx     context.Context
	cfg     *KafkaController
	cfgFile string
	ticker  *time.Ticker
	stop    chan struct{}
	proc    *os.Process
	cancel  context.CancelFunc
}

func NewKafkaSM(ctx context.Context, cfg KafkaController) *KafkaSM {
	sm := stateless.NewStateMachine(string(StateInit))
	k := &KafkaSM{sm: sm, ctx: ctx, cfg: &cfg}
	k.configure()
	k.sm.ActivateCtx(ctx)
	return k
}

func (k *KafkaSM) configure() {
	k.sm.Configure(string(StateInit)).
		Permit(string(TrigStart), string(StateFormatting))

	k.sm.Configure(string(StateFormatting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			deleted, err := kafkautil.CleanKRaftDeletedFiles(os.Getenv("KAFKA_META_DIR"))
			if err != nil {
				log.Println("Error cleaning deleted files:", err)
			} else {
				log.Println("Deleted files:", deleted)
			}
			log.Println("stateFormat")
			s := args[0].(KafkaReplSetSpec)
			if err := k.formatStorage(s); err != nil {
				return err
			} else {
				return k.sm.FireCtx(ctx, string(TrigStartServer), args...)
			}

		}).
		Permit(string(TrigStartServer), string(StateStartup))

	k.sm.Configure(string(StateStarting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			log.Println("starting state")
			if err := k.startKafkaProcess(); err != nil {
				return k.sm.FireCtx(ctx, string(TrigKafkaExit), err)
			}
			return k.startMonitor()
		}).
		OnExit(func(ctx context.Context, args ...any) error {
			log.Println("starting state exiting")
			return k.stopMonitor()
		}).
		Permit(string(TrigKafkaExit), string(StateFailed))

	k.sm.Configure(string(StateStartup)).
		SubstateOf(string(StateStarting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			log.Println("state starting.startup")
			return nil
		}).
		Permit(string(TrigReady), string(StateRunning)).
		Permit(string(TrigShutdown), string(StateShuttingDown))

	k.sm.Configure(string(StateRunning)).
		SubstateOf(string(StateStarting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			log.Println("kafka proc started")
			if err := waitKafkaReady(ctx, args...); err != nil {
				return err
			}
			return k.sm.FireCtx(ctx, string(TrigDone), args...)
		}).
		Permit(string(TrigShutdown), string(StateShuttingDown))

	k.sm.Configure(string(StateShuttingDown)).
		SubstateOf(string(StateStarting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			log.Println("state starting.shuttingdown shuting down")
			log.Println("sending break")
			k.cancel()
			return nil
		}).
		Permit(string(TrigDone), string(StateRunning))

	k.sm.Configure(string(StateFailed)).
		OnEntry(func(ctx context.Context, args ...any) error {
			log.Println("KAFKA ERROR")
			os.Exit(0)
			return nil
		})
}

func (k *KafkaSM) stopMonitor() error {
	log.Println("stopping monitoring")
	close(k.stop)
	return nil
}

func (k *KafkaSM) startMonitor() error {
	log.Println("startin monitor")
	k.ticker = time.NewTicker(5 * time.Second)
	k.stop = make(chan struct{})

	go func() {
		defer k.ticker.Stop()
		defer log.Println("stopped monitor")
		for {
			log.Println("Connecting kafka")
			conn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%s", k.cfg.cfg.BrokerAddr, k.cfg.cfg.BrokerPort))
			if err != nil {
				k.cfg.healthChan <- KafkaHealthStatus{
					NodeName: k.cfg.cfg.NodeName,
					NodeId:   k.cfg.cfg.NodeID,
					Status:   "Disconnected",
				}
				log.Println("kafka dial error")
				time.Sleep(3 * time.Second)
				continue
			}
			log.Print("Connected to kafka broker")
			for {
				select {
				case <-k.ticker.C:
					log.Println("tick event")

					if brokers, err := conn.Brokers(); err != nil {
						log.Println("kafka brokers error")
						k.cfg.healthChan <- KafkaHealthStatus{
							NodeName: k.cfg.cfg.NodeName,
							NodeId:   k.cfg.cfg.NodeID,
							Status:   "BrokerErr",
						}
						time.Sleep(3 * time.Second)
						break
					} else {
						log.Println("Brokers", brokers)
					}
					if controller, err := conn.Controller(); err != nil {
						log.Println("kafka brokers error")
						k.cfg.healthChan <- KafkaHealthStatus{
							NodeName: k.cfg.cfg.NodeName,
							NodeId:   k.cfg.cfg.NodeID,
							Status:   "ControllerErr",
						}
						time.Sleep(3 * time.Second)
						break
					} else {
						log.Println("Controller", controller)
					}
					k.cfg.healthChan <- KafkaHealthStatus{
						NodeName: k.cfg.cfg.NodeName,
						NodeId:   k.cfg.cfg.NodeID,
						Status:   "OK",
					}

				case <-k.stop:
					log.Println("stop received. exiting monitoring")
					return
				}
			}
		}
	}()
	return nil
}

func (k *KafkaSM) createConfig(s KafkaReplSetSpec) (string, error) {

	alloc := normalize(os.Getenv("NOMAD_ALLOC_DIR"))
	cfgFile := normalize(filepath.Join(alloc, "server.properties"))

	props := &ServerProperties{
		ID:               k.cfg.cfg.NodeID,
		ControllerAddr:   fmt.Sprintf("%s:%s", k.cfg.cfg.ControllerAddr, k.cfg.cfg.ControllerPort),
		BrokerAddr:       fmt.Sprintf("%s:%s", k.cfg.cfg.BrokerAddr, k.cfg.cfg.BrokerPort),
		MetaLogDir:       normalize(k.cfg.cfg.MetaDir),
		LogDir:           normalize(k.cfg.cfg.DatDir),
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
func (k *KafkaSM) formatStorage(s KafkaReplSetSpec) error {
	log.Println("format storage", k, s)

	cfg, err := k.createConfig(s)
	if err != nil {
		panic(err)
	}

	log.Println("Cfg file;", cfg)
	cfgFile := normalize(cfg)
	k.cfgFile = cfgFile
	log.Println("normalized Cfg file;", cfg)
	env := baseEnv()
	env = append(env, fmt.Sprintf("LOG_DIR=%s", normalize(filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "kafka-logs"))))
	procArgs := []string{"/c", "kafka-storage.bat", "format", "-t", s.ClusterID, "-c", cfgFile, "--initial-controllers", s.BootstrapServersStorage, "--ignore-formatted"}
	log.Println("Runnig kafka-storage.bat with:", procArgs)

	cmd := exec.Command("cmd", procArgs...)
	cmd.Env = env

	if out, err := cmd.CombinedOutput(); err != nil {
		log.Println("storage out:", string(out))
		return err
	} else {
		log.Println("storage out:", string(out))
	}
	log.Println("storage exit status:", cmd.ProcessState)
	return nil
}

func (k *KafkaSM) startKafkaProcess() error {
	ctx, stop := context.WithCancel(context.Background())
	k.cancel = stop
	log.Println("starting kafka-server process")
	env := baseEnv()
	env = append(env, fmt.Sprintf("LOG_DIR=%s", normalize(filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "kafka-logs"))))
	cmd := exec.Command("cmd", "/C", "kafka-server-start.bat", k.cfgFile)

	cmd = exec.CommandContext(ctx, "java", "-Xmx1G", "-Xms1G", "-server", "-XX:+UseG1GC", "-XX:MaxGCPauseMillis=20", "-XX:InitiatingHeapOccupancyPercent=35",
		"-XX:+ExplicitGCInvokesConcurrent", "-Djava.awt.headless=true",
		fmt.Sprintf("-Dkafka.logs.dir=%s", filepath.Join(os.Getenv("KAFKA_TOOLS"), "../../config/log4j2.yaml")),
		"-cp", fmt.Sprintf("%s", filepath.Join(os.Getenv("KAFKA_TOOLS"), "../../libs/*")),
		"kafka.Kafka",
		k.cfgFile,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	// -Xmx1G -Xms1G -server -XX:+UseG1GC -XX:MaxGCPauseMillis=20 -XX:InitiatingHeapOccupancyPercent=35 -XX:+ExplicitGCInvokesConcurrent -Djava.awt.headless=true -Dkafka.logs.dir="D:\works\mongoctl\logs" "-Dlog4j2.configurationFile=D:\works\mongoctl\cots\kafka\bin\windows\../../config/log4j2.yaml"
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}
	k.proc = cmd.Process
	log.Println("PID:", k.proc.Pid)

	// graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		sig := <-sigs
		log.Println("Signal:", sig)
		log.Println(k.sm.State(k.ctx))
		k.sm.FireCtx(k.ctx, string(TrigShutdown))
	}()
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Println("kafka exited with error", cmd.ProcessState)
		}
		log.Println("Kafka exited", cmd.ProcessState)
		k.sm.FireCtx(k.ctx, string(TrigKafkaExit), cmd.ProcessState)
	}()

	return nil
}

func waitKafkaReady(ctx context.Context, args ...any) error {
	log.Println("wait ready", args)
	time.Sleep(1 * time.Second)
	return nil
}
func monitorKafka(ctx context.Context, args ...any) error {
	log.Println("monitor kafka", args)
	time.Sleep(1 * time.Second)
	return nil
}
func (k *KafkaSM) FireExit(args ...any) error {
	return k.sm.FireCtx(k.ctx, string(TrigKafkaExit), args...)
}
func (k *KafkaSM) FireStart(args ...any) error {
	log.Println("send start", args)
	return k.sm.FireCtx(k.ctx, string(TrigStart), args...)
}

var (

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
default.replication.factor=2
min.insync.replicas=2
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
		}, ";")),
		"CLASSPATH=",
	)
}
