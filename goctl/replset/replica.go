package replset

import (
	"log"
	"slices"
	"time"

	"example.com/goctl/store"
)

type ReplicaSetStatus int
type controllerInterface[C, S, H any] interface {
	collect() (*C, error)
	generateReplConfig([]C) *S
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
	ControllerTask()
	MemberTask()
}

// replicaSetControl definition remains unchanged
type replicaSetControl[
	C store.CandidateReportType[C],
	S any,
	H store.HealtStatusType[H],
] struct {
	ReplicaController
	collector    controllerInterface[C, S, H]
	name         string
	replConfig   *S
	reports      []C
	healthStatus []H
	store        store.Provider[C, S, H]
	ch           chan string
	state        ReplicaSetStatus
	timer        *time.Timer
	candidates   []C
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
	H]) ControllerTask() {
	rs.timer = time.NewTimer(5 * time.Minute)
	candidatesChan := rs.store.WatchCandidateReports()
	healthStatusChan := rs.store.WatchHealthStatus()
	for {
		select {
		case candidateReports := <-candidatesChan:
			rs.handleCandidates(candidateReports)
		case healthStatus := <-healthStatusChan:
			rs.handleHealthStatus(healthStatus)
		case t := <-rs.timer.C:
			rs.handleTimer(t)
		}
	}
}
func (rs *replicaSetControl[
	C,
	S,
	H]) MemberTask() {
	configChan := rs.store.WatchReplSetConfig()
	rs.collector.memberTask(configChan)
}
func (rs *replicaSetControl[
	C,
	S,
	H]) handleCandidates(candidates []C) {
	log.Println(candidates)
	numOfCandidates := len(candidates)
	rs.candidates = candidates
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
			rs.timer = time.NewTimer(10 * time.Second)
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
		log.Println("Monitor state. wil be implemented")
	}
}

func (rs *replicaSetControl[
	C,
	S,
	H,
]) handleHealthStatus(healthStatus []H) {
	log.Println(healthStatus)
}
func (rs *replicaSetControl[C, S, H]) handleTimer(t time.Time) {
	log.Println("timer", t)
	if rs.state == INITIATION {
		panic("Replset Configuration timeout")
	} else if rs.state == CONFIGURATION {
		log.Println("here we publish initial configuration")
		sortCandidates(rs.candidates)
		replCfg := rs.collector.generateReplConfig(rs.candidates)
		err := rs.store.UpdateReplSetConfig(replCfg)
		if err != nil {
			return
		}
		//rs.

	}
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
