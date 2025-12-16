package replset

import (
	"context"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"example.com/goctl/internal/store"
	napi "github.com/hashicorp/nomad/api"
	"github.com/qmuntal/stateless"
)

type ReplicaSetStatus int
type controllerInterface[C, S, H any] interface {
	collect() (*C, error)
	generateReplConfig([]C) S
	memberTask(<-chan S) <-chan H
}

const (
	INITIATION ReplicaSetStatus = iota
	CONFIGURATION
	STARTUP
	MONITOR
	ERROR
)

type ReplicaController interface {
	PreStartTask(id string)
	ControllerTask(string)
	MemberTask(string)
}

// replicaSetControl definition remains unchanged
type replicaSetControl[
	C store.CandidateReportType[C],
	S store.ReplicaSetSpecType[S],
	H store.HealtStatusType[H],
] struct {
	ReplicaController
	collector   controllerInterface[C, S, H]
	name        string
	store       store.Provider[C, S, H]
	ch          chan string
	state       ReplicaSetStatus
	timer       *time.Timer
	candidates  []C
	lastMembers []C
	lastSpec    *S
	jobFile     string
	nomadAddr   string
}

func (rs *replicaSetControl[
	C,
	S,
	H,
]) PreStartTask(id string) {
	res, err := rs.collector.collect()
	if err != nil {
		panic(err)
	}
	err = rs.store.PutCandidateReport(id, res)
	if err != nil {
		panic(err)
	}

}

func (rs *replicaSetControl[
	C,
	S,
	H]) ControllerTask(jobFile string) {
	rs.jobFile = jobFile

	replCtrl := stateless.NewStateMachine(stateInit)

	replCtrl.Configure(stateInit).
		Permit(trigerStart, stateConfiguration)

	replCtrl.Configure(stateConfiguration).
		OnEntry(func(ctx context.Context, args ...any) error {
			if rs.timer != nil {
				rs.timer.Stop()
				rs.timer = nil
			}
			if len(args) == 0 {
				return nil
			}
			log.Println("config", args)
			candidates, ok := args[0].([]C)
			if !ok {
				rs.candidates = candidates
			}
			log.Println(candidates)
			numOfCandidates := len(candidates)
			if numOfCandidates == 0 {
				return nil
			}
			if numOfCandidates == 6 {
				log.Println("initiation done with full members")
				rs.timer = time.AfterFunc(time.Second*1, func() {
					replCtrl.FireCtx(context.Background(), triggerInitialConfiguration, candidates)
				})
			} else if numOfCandidates >= 3 {
				log.Println("wait a while to others")
				rs.timer = time.AfterFunc(time.Second*5, func() {
					replCtrl.FireCtx(context.Background(), triggerInitialConfiguration, candidates)
				})
			} else if numOfCandidates > 0 {
				log.Println("wait 15s for just one member")
				//This is worst case
				rs.timer = time.AfterFunc(time.Second*15, func() {
					replCtrl.FireCtx(context.Background(), triggerInitialConfiguration, candidates)
				})
			}

			return nil
		}).
		Permit(triggerInitialConfiguration, stateMonitoring).
		PermitReentry(triggerCandidateReport)

	replCtrl.Configure(stateMonitoring).
		OnEntryFrom(triggerInitialConfiguration, func(ctx context.Context, args ...any) error {
			candidates := args[0].([]C)
			log.Println("deploying initial configuration", candidates)
			sortCandidates(candidates)
			length := 3
			if len(candidates) < 3 {
				length = len(rs.candidates)
			}
			replCfg := rs.collector.generateReplConfig(candidates[0:length])
			err := rs.publishReplSpec(replCfg)
			if err != nil {
				log.Println("Error publishing repl spec:", err)
				return err
			}
			rs.lastMembers = candidates[0:length]
			// save last spec and move to monitor state
			rs.lastSpec = &replCfg
			return nil
		}).
		OnEntryFrom(triggerCandidateReport, func(ctx context.Context, args ...any) error {

			log.Println("candi report", args)
			return nil
		}).
		OnEntryFrom(triggerMemberStatus, func(ctx context.Context, args ...any) error {
			log.Println("health status", args)
			return nil
		}).
		PermitReentry(triggerMemberStatus).
		PermitReentry(triggerCandidateReport)

	replCtrl.ActivateCtx(context.Background())

	replCtrl.FireCtx(context.Background(), trigerStart)

	rs.timer = time.NewTimer(5 * time.Minute)
	candidatesChan := rs.store.WatchCandidateReports()
	healthStatusChan := rs.store.WatchHealthStatus()
	for {
		select {
		case candidateReports := <-candidatesChan:
			log.Println("candi rep", candidateReports)
			replCtrl.FireCtx(context.Background(), triggerCandidateReport, candidateReports)
			log.Println("End")
		case healthStatus := <-healthStatusChan:
			log.Println("health status", healthStatus)
			replCtrl.Fire(context.Background(), triggerMemberStatus, healthStatus)
		}
	}
}
func (rs *replicaSetControl[
	C,
	S,
	H]) MemberTask(id string) {
	healthCh := rs.collector.memberTask(rs.store.WatchReplSetConfig())
	for health := range healthCh {
		rs.store.UpdateHealthStatus(id, health)
	}
	log.Println("Done member task")
}
func (rs *replicaSetControl[
	C,
	S,
	H]) handleCandidates(candidates []C) {
	log.Println(candidates)
	numOfCandidates := len(candidates)
	if numOfCandidates == 0 {
		return
	}
	rs.candidates = candidates
	candidates[0].GetId()
	if rs.state == INITIATION {
		if numOfCandidates == 6 {
			log.Println("initiation done with full members")
			rs.timer = time.NewTimer(1 * time.Second)
			rs.state = CONFIGURATION
		} else if numOfCandidates >= 3 {
			log.Println("wait a while to others")
			rs.timer = time.NewTimer(5 * time.Second)
			rs.state = CONFIGURATION
		} else if numOfCandidates > 0 {
			//This is worst case. we have members to initiate replicaset
			rs.timer = time.NewTimer(15 * time.Second)
			rs.state = CONFIGURATION
			log.Println("")
		}

	} else if rs.state == CONFIGURATION {
		if numOfCandidates == 0 {
			log.Println("we'v lost members in configuration state. Start again")
			rs.timer = time.NewTimer(5 * time.Minute)
			rs.state = INITIATION
		}
	} else if rs.state == MONITOR {
		// Candidate list updated while monitoring; nothing else to do here
		log.Println("Monitor: candidate list updated")
	}
}

func (rs *replicaSetControl[
	C,
	S,
	H,
]) handleHealthStatus(healthStatus []H) {
	log.Println("healthStatus", healthStatus)
}
func (rs *replicaSetControl[C, S, H]) handleTimer(t time.Time) {
	log.Println("timer", t)
	if rs.state == INITIATION {
		panic("Replset Configuration timeout")
	} else if rs.state == CONFIGURATION {
		log.Println("here we publish initial configuration")
		sortCandidates(rs.candidates)
		length := 3
		if len(rs.candidates) < 3 {
			length = len(rs.candidates)
		}
		replCfg := rs.collector.generateReplConfig(rs.candidates[0:length])
		err := rs.publishReplSpec(replCfg)
		if err != nil {
			log.Println("Error publishing repl spec:", err)
			return
		}
		rs.lastMembers = rs.candidates[0:length]
		// save last spec and move to monitor state
		rs.lastSpec = &replCfg
		rs.state = MONITOR

		//rs.

	}
}
func (rs *replicaSetControl[C, S, H]) publishReplSpec(spec S) error {

	log.Println("Publishing new replset configuration")
	err := rs.store.UpdateReplSetConfig(&spec)
	if err != nil {
		return err
	}
	members := spec.GetMembers()
	vars := []string{
		fmt.Sprintf("replica-count=%d", len(members)),
		fmt.Sprintf("replica-members=\"%s\"", strings.Join(members, ",")),
	}
	log.Println("vars", vars)

	client, err := napi.NewClient(&napi.Config{
		Address:  os.Getenv("NOMAD_ADDR"),
		WaitTime: 5 * time.Second,
	})
	if err != nil {
		return err
	}
	varsStr := strings.Join(vars, "\n")
	log.Println("varStr", varsStr)
	hclStr, err := os.ReadFile(rs.jobFile)
	hcl, err := client.Jobs().ParseHCLOpts(&napi.JobsParseRequest{
		JobHCL:       string(hclStr),
		Variables:    varsStr,
		Canonicalize: false,
	})
	if err != nil {
		return err
	}
	_, _, err = client.Jobs().Register(hcl, nil)
	if err != nil {
		return err
	}
	log.Println("Job deploy succeed")
	return nil

}

func sortCandidates[T store.Orderable[T]](candidates []T) {
	slices.SortFunc(candidates, func(a, b T) int {
		switch {
		case a.Less(b):
			return -1
		case b.Less(a):
			return 1
		default:
			return 0
		}
	})
}

func test() {
}
