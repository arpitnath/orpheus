package service

import (
	"context"
	"testing"
)

func TestCommand_Creation(t *testing.T) {
	ctx := context.Background()
	responseCh := make(chan error, 1)
	notifyCh := make(chan error, 1)

	cmd := Command{
		Action:      CmdEnsureRunning,
		Ctx:         ctx,
		Response:    responseCh,
		TargetState: StateReady,
		NotifyChan:  notifyCh,
	}

	if cmd.Action != CmdEnsureRunning {
		t.Errorf("Expected CmdEnsureRunning, got %v", cmd.Action)
	}

	if cmd.Ctx != ctx {
		t.Error("Context not preserved")
	}

	if cmd.TargetState != StateReady {
		t.Errorf("Expected StateReady, got %v", cmd.TargetState)
	}
}

func TestCommandAction_Values(t *testing.T) {
	// Verify iota values are distinct
	if CmdEnsureRunning == CmdStop {
		t.Error("CmdEnsureRunning and CmdStop should be different")
	}

	if CmdStop == CmdSubscribe {
		t.Error("CmdStop and CmdSubscribe should be different")
	}

	if CmdEnsureRunning == CmdSubscribe {
		t.Error("CmdEnsureRunning and CmdSubscribe should be different")
	}

	// Verify expected values
	if CmdEnsureRunning != 0 {
		t.Errorf("CmdEnsureRunning should be 0 (first iota), got %d", CmdEnsureRunning)
	}

	if CmdStop != 1 {
		t.Errorf("CmdStop should be 1, got %d", CmdStop)
	}

	if CmdSubscribe != 2 {
		t.Errorf("CmdSubscribe should be 2, got %d", CmdSubscribe)
	}
}

func TestStateSubscription_Creation(t *testing.T) {
	notifyCh := make(chan error, 1)

	sub := StateSubscription{
		TargetState: StateReady,
		NotifyChan:  notifyCh,
	}

	if sub.TargetState != StateReady {
		t.Errorf("Expected StateReady, got %v", sub.TargetState)
	}

	if sub.NotifyChan == nil {
		t.Error("NotifyChan should not be nil")
	}
}

func TestCommand_ResponseChannel(t *testing.T) {
	responseCh := make(chan error, 1)

	cmd := Command{
		Action:   CmdEnsureRunning,
		Response: responseCh,
	}

	// Simulate sending response
	go func() {
		cmd.Response <- nil
	}()

	// Receive response
	err := <-cmd.Response
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
}

func TestStateSubscription_Notification(t *testing.T) {
	notifyCh := make(chan error, 1)

	sub := StateSubscription{
		TargetState: StateReady,
		NotifyChan:  notifyCh,
	}

	// Simulate notification
	go func() {
		sub.NotifyChan <- nil
	}()

	// Receive notification
	err := <-sub.NotifyChan
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
}
