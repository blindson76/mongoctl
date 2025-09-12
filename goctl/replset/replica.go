package replset

import (
	"log"
	"slices"
	"time"
)

type ReplicaSetStatus int
type Unique[T any] interface {
	GetId() string
}

type Orderable[T any] interface {
	Less(other T) bool
}

type PreStartController[C any] interface {
	Collect() (C, error)
}

const (
	INITIATION ReplicaSetStatus = iota
	CONFIGURATION
	STARTUP
	MONITOR
	ERROR
)

type CandidateReportType[C any] interface {
	Orderable[C]
	Unique[C]
}
type HealtStatusType[H any] interface {
	Orderable[H]
	Unique[H]
	IsHealthy() bool
}
type ReplicaSetSpecType[S any] interface {
	ApplyConfig() error
}

type StoreBackend[C, S, H any] interface {
	PutCandidateReport(Val C) error
	WatchCandidateReports() <-chan []C
	UpdateHealthStatus(status H) error
	WatchHealthStatus() <-chan []H
	UpdateReplSetConfig(cfg S) error
	WatchReplSetConfig() <-chan S
}

// ReplicaSetController definition remains unchanged
type ReplicaSetController[
	C CandidateReportType[C],
	S ReplicaSetSpecType[S],
	H HealtStatusType[H],
] struct {
	PreStartController[C]
	name         string
	replConfig   *S
	reports      []C
	healthStatus []H
	store        StoreBackend[C, S, H]
	ch           chan string
	state        ReplicaSetStatus
	timer        *time.Timer
	candidates   []C
}

func (rs *ReplicaSetController[
	C,
	S,
	H,
]) PreStartTask(id string) {
	res, err := rs.Collect()
	if err != nil {
		panic(err)
	}
	log.Println(res)

}

func (rs *ReplicaSetController[
	C,
	S,
	H]) ControllerTask() {
	rs.timer = time.NewTimer(5 * time.Minute)
	for {
		select {
		case candidateReports := <-rs.store.WatchCandidateReports():
			rs.handleCandidates(candidateReports)
		case healthStatus := <-rs.store.WatchHealthStatus():
			rs.handleHealthStatus(healthStatus)
		case t := <-rs.timer.C:
			rs.handleTimer(t)
		}
	}
}
func (rs *ReplicaSetController[
	C,
	S,
	H]) MemberTask() {

	var cfg S
	cfg.ApplyConfig()
}
func (rs *ReplicaSetController[
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
			rs.timer = time.NewTimer(10 * time.Second)
			rs.state = CONFIGURATION
		} else if numOfCandidates > 0 {
			//This is worst case. we have members to initiate replicaset
			rs.timer = time.NewTimer(1 * time.Minute)
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

func (rs *ReplicaSetController[
	C,
	S,
	H,
]) handleHealthStatus(healthStatus []H) {
	log.Println(healthStatus)
}
func (rs *ReplicaSetController[C, S, H]) handleTimer(t time.Time) {
	log.Println("timer", t)
	if rs.state == INITIATION {
		panic("Replset Configuration timeout")
	} else if rs.state == CONFIGURATION {
		log.Println("here we publish initial configuration")

	}
}

func sortCandidates[T Orderable[T]](candidates []T) {
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

type MongoStatusReport struct {
}

func (m MongoStatusReport) Less(o MongoStatusReport) bool {
	return false
}

func (m MongoStatusReport) GetId() string {
	return "0"
}

type MongoReplSetSpec struct {
}

type MongoHealthStatus struct {
}

func (m MongoHealthStatus) IsHealthy() bool {
	return true
}

func (m MongoHealthStatus) GetId() string {
	return "0"
}

func test() {
}
