package util

// KafkaMgm provides lightweight cluster/quorum helpers for KRaft-backed Kafka
// using a combination of filesystem markers (for test/dev orchestration) and
// invoking Windows Kafka scripts (if available) for runtime checks.
type KafkaMgm struct {
	// DataDir is the root directory where per-node kafka data directories live.
	// Each member is represented by a subdirectory under DataDir (e.g. DataDir/1).
	DataDir string
	// KafkaBin points to the Kafka windows bin dir (where kafka-broker-api-versions.bat,
	// kafka-storage.bat, kafka-server-start.bat etc. live). If empty, script calls
	// will be skipped and only file-backed operations are used.
	KafkaBin string
	// DefaultBootstrap is an optional bootstrap server (host:port) used for health checks.
	DefaultBootstrap string
}
