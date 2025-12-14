module example.com/goctl

go 1.24.6

require (
	github.com/hashicorp/consul/api v1.32.1
	github.com/hashicorp/nomad/api v0.0.0-20250919063558-6ea57a589d80
	github.com/qmuntal/stateless v1.7.2
	github.com/segmentio/kafka-go v0.4.49
	github.com/spf13/viper v1.20.1
	go.mongodb.org/mongo-driver/v2 v2.3.0
)

replace (
	go.uber.org/atomic v1.9.0 => github.com/uber-go/atomic v1.9.0
	go.uber.org/goleak v1.1.10 => github.com/uber-go/goleak v1.1.10
	go.uber.org/multierr v1.9.0 => github.com/uber-go/multierr v1.9.0
	golang.org/x/crypto => github.com/golang/crypto v0.33.0
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 => github.com/golang/exp v0.0.0-20250305212735-054e65f0b394
	golang.org/x/lint => github.com/golang/lint v0.0.0-20190930215403-16217165b5de
	golang.org/x/mod => github.com/golang/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4
	golang.org/x/net => github.com/golang/net v0.0.0-20190613194153-d28f0bde5980
	golang.org/x/sync => github.com/golang/sync v0.0.0-20181108010431-42b317875d0f
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e => github.com/golang/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sync v0.12.0 => github.com/golang/sync v0.12.0
	golang.org/x/sys => github.com/golang/sys v0.31.0
	golang.org/x/sys v0.0.0-20190422165155-953cdadca894 => github.com/golang/sys v0.0.0-20190422165155-953cdadca894
	golang.org/x/sys v0.0.0-20200122134326-e047566fdf82 => github.com/golang/sys v0.0.0-20200122134326-e047566fdf82
	golang.org/x/sys v0.0.0-20220503163025-988cb79eb6c6 => github.com/golang/sys v0.0.0-20220503163025-988cb79eb6c6
	golang.org/x/sys v0.0.0-20220728004956-3c1f35247d10 => github.com/golang/sys v0.0.0-20220728004956-3c1f35247d10
	golang.org/x/sys v0.6.0 => github.com/golang/sys v0.6.0
	golang.org/x/term => github.com/golang/term v0.29.0
	golang.org/x/text => github.com/golang/text v0.3.8
	golang.org/x/text v0.23.0 => github.com/golang/text v0.23.0
	golang.org/x/tools => github.com/golang/tools v0.0.0-20191108193012-7d206e10da11
	golang.org/x/xerrors => github.com/golang/xerrors v0.0.0-20191204190536-9bdfabe68543
)

require (
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/cronexpr v1.1.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
