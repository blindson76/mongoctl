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
	"time"

	"github.com/qmuntal/stateless"
	"github.com/segmentio/kafka-go"
)

type State string
type Trigger string

const (
	StateInit       State = "init"
	StateFormatting State = "formatting"
	StateStarting   State = "starting"
	StateFailed     State = "failed"

	StateStartup    State = "startup"
	StateStarted    State = "started"
	StateMonitoring State = "monitoring"
)

const (
	TrigStart       Trigger = "start"
	TrigStartServer Trigger = "startServer"
	TrigDone        Trigger = "done"
	TrigKafkaExit   Trigger = "kafkaExit"
)

type KafkaSM struct {
	sm      *stateless.StateMachine
	ctx     context.Context
	cfg     *KafkaController
	cfgFile string
}

func NewKafkaSM(ctx context.Context, cfg KafkaController) *KafkaSM {
	sm := stateless.NewStateMachine(string(stateInit))
	k := &KafkaSM{sm: sm, ctx: ctx, cfg: &cfg}
	k.configure()
	k.sm.ActivateCtx(ctx)
	return k
}

func (k *KafkaSM) configure() {
	k.sm.Configure(string(stateInit)).
		Permit(string(TrigStart), string(StateFormatting))

	k.sm.Configure(string(StateFormatting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			log.Println("stateFormat")
			s := args[0].(KafkaReplSetSpec)
			if err := k.formatStorage(s); err != nil {
				return err
			} else {
				return k.sm.FireCtx(ctx, string(TrigStartServer), args...)
			}

		}).
		Permit(string(TrigStartServer), string(StateStartup))

	k.sm.Configure(StateStarting).
		Permit(TrigKafkaExit, StateFailed)

	k.sm.Configure(string(StateStartup)).
		SubstateOf(string(StateStarting)).
		OnEntry(func(ctx context.Context, args ...any) error {

			if err := k.startKafkaProcess(); err != nil {
				return err
			}
			return k.sm.FireCtx(ctx, string(TrigDone), args...)
		}).
		Permit(string(TrigDone), string(StateStarted))

	k.sm.Configure(string(StateStarted)).
		SubstateOf(string(StateStarting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			if err := waitKafkaReady(ctx, args...); err != nil {
				return err
			}
			return k.sm.FireCtx(ctx, string(TrigDone), args...)
		}).
		Permit(string(TrigDone), string(stateMonitoring))

	k.sm.Configure(string(stateMonitoring)).
		SubstateOf(string(StateStarting)).
		OnEntry(func(ctx context.Context, args ...any) error {
			return monitorKafka(ctx, args...)
		}).
		Permit(string(TrigDone), string(StateStarted))

	k.sm.Configure(string(StateFailed))
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
	log.Println("proc state:", cmd.ProcessState)
	return nil
}

func (k *KafkaSM) startKafkaProcess() error {
	log.Println("start proess")
	env := baseEnv()
	env = append(env, fmt.Sprintf("LOG_DIR=%s", normalize(filepath.Join(os.Getenv("NOMAD_ALLOC_DIR"), "kafka-logs"))))
	cmd := exec.Command("kafka-server-start.bat", k.cfgFile)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	exitChan := make(chan *os.ProcessState, 1)

	if err := cmd.Start(); err != nil {
		return err
	}

	// graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		sig := <-sigs
		log.Println("Signal:", sig)
		_ = cmd.Process.Signal(os.Kill)
		time.Sleep(5 * time.Second)
		log.Println("killing cmd")
		_ = cmd.Process.Kill()
		os.Exit(0)
	}()
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Println("kafka exited with error", cmd.ProcessState)
		}
		log.Println("Kafka exited", cmd.ProcessState)
		exitChan <- cmd.ProcessState
	}()

	go func() {
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
			for {

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
				time.Sleep(5 * time.Second)
			}
		}
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
