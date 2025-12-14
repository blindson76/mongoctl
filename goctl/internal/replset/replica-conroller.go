package replset

import (
	"context"
	"fmt"
	"log"

	"github.com/qmuntal/stateless"
)

const (
	trigerStart                 = "start"
	triggerCandidateReport      = "candidateReport"
	triggerMemberStatus         = "memberStatus"
	triggerTimeout              = "timeout"
	triggerInitialConfiguration = "initialConfiguration"
)

const (
	stateInit          = "init"
	stateConfiguration = "configuration"
	stateMonitoring    = "monitoring"
	stateShuttingDown  = "shuttingDown"
	stateTerminated    = "terminated"
)

type RepllicaSM struct {
}

func main() {

	replCtrl := stateless.NewStateMachine(stateInit)

	replCtrl.Configure(stateInit).
		Permit(trigerStart, stateConfiguration)

	replCtrl.Configure(stateConfiguration).
		Permit(triggerInitialConfiguration, stateMonitoring).
		PermitReentry(triggerCandidateReport)

	replCtrl.Configure(stateMonitoring).
		OnEntry(func(ctx context.Context, args ...any) error {
			log.Println("monitor")
			return nil
		})

	replCtrl.Activate()

	replCtrl.Fire(trigerStart, 5)
	replCtrl.Fire(triggerCandidateReport, "deneme")
	replCtrl.Fire(triggerInitialConfiguration)

	fmt.Println("Hello, World!", stateConfiguration)
}
