package service

import "context"

type CommandAction int

const (
	CmdEnsureRunning CommandAction = iota
	CmdStop
	CmdSubscribe
)

type Command struct {
	Action      CommandAction
	Ctx         context.Context
	Response    chan error
	TargetState ServerState
	NotifyChan  chan error
}

type StateSubscription struct {
	TargetState ServerState
	NotifyChan  chan error
}
