package util

import "go.mongodb.org/mongo-driver/v2/bson"

type Settings struct {
	ReplicaSetId bson.ObjectID `bson:"replicaSetId"`
}
type Member struct {
	ID   int    `bson:"_id"`
	Host string `bson:"host"`
}
type Replset struct {
	ID       string   `bson:"_id"`
	Members  []Member `bson:"members"`
	Term     int64    `bson:"term"`
	Settings Settings `bson:"settings"`
	Version  int      `bson:"version"`
}

type OpLog struct {
	Ts bson.Timestamp `bson:"ts"`
}

type MongoStatusMap map[string]*MongoStatus

type MongoStatus struct {
	NodeId        string
	NodeName      string
	ReplSetId     string
	ReplSetName   string
	Members       string
	Term          int64
	OpLogFirstSec uint32
	OpLogFirstInc uint32
	OpLogLastSec  uint32
	OpLogLastInc  uint32
	MongoAddr     string
	MongoPort     string
}

type ReplSetConfig struct {
	Config Replset `bson:"config"`
}
type Optime struct {
	Ts bson.Timestamp `bson:"ts"`
	T  int64          `bson:"t"`
}
type Optimes struct {
	LastCommittedOpTime       Optime `bson:"lastCommittedOpTime"`
	ReadConcernMajorityOpTime Optime `bson:"readConcernMajorityOpTime"`
	AppliedOpTime             Optime `bson:"appliedOpTime"`
	WrittenOpTime             Optime `bson:"writtenOpTime"`
	DurableOpTime             Optime `bson:"durableOpTime"`
}
type MongoStatusEnum int

const (
	MongoStatus_STARTUP    MongoStatusEnum = 0
	MongoStatus_PRIMARY    MongoStatusEnum = 1
	MongoStatus_SECONDARY  MongoStatusEnum = 2
	MongoStatus_RECOVERING MongoStatusEnum = 3
	MongoStatus_STARTUP2   MongoStatusEnum = 5
	MongoStatus_UNKNOWN    MongoStatusEnum = 6
	MongoStatus_ARBITER    MongoStatusEnum = 7
	MongoStatus_DOWN       MongoStatusEnum = 8
	MongoStatus_ROLLBACK   MongoStatusEnum = 9
	MongoStatus_REMOVED    MongoStatusEnum = 10
)

func (e MongoStatusEnum) String() string {
	switch e {
	case MongoStatus_STARTUP:
		return "STARTUP"
	case MongoStatus_PRIMARY:
		return "PRIMARY"
	case MongoStatus_SECONDARY:
		return "SECONDARY"
	case MongoStatus_RECOVERING:
		return "RECOVERING"
	case MongoStatus_STARTUP2:
		return "STARTUP2"
	case MongoStatus_UNKNOWN:
		return "UNKNOWN"
	case MongoStatus_ARBITER:
		return "ARBITER"
	case MongoStatus_DOWN:
		return "DOWN"
	case MongoStatus_ROLLBACK:
		return "ROLLBACK"
	case MongoStatus_REMOVED:
		return "REMOVED"
	}
	return ""
}

type ReplSetStatus struct {
	Set     string          `bson:"set"`
	Term    int64           `bson:"term"`
	Optimes Optimes         `bson:"optimes"`
	MyState MongoStatusEnum `bson:"myState"`
}
