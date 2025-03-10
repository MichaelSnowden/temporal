// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

//go:generate mockgen -copyright_file ../../../LICENSE -package $GOPACKAGE -source $GOFILE -destination workflow_task_state_machine_mock.go

package workflow

import (
	"fmt"
	"math"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	failurepb "go.temporal.io/api/failure/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"

	enumsspb "go.temporal.io/server/api/enums/v1"
	historyspb "go.temporal.io/server/api/history/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/primitives/timestamp"
)

type (
	workflowTaskStateMachine struct {
		ms *MutableStateImpl
	}
)

const (
	workflowTaskRetryBackoffMinAttempts = 3
	workflowTaskRetryInitialInterval    = 5 * time.Second
)

func newWorkflowTaskStateMachine(
	ms *MutableStateImpl,
) *workflowTaskStateMachine {
	return &workflowTaskStateMachine{
		ms: ms,
	}
}

func (m *workflowTaskStateMachine) ReplicateWorkflowTaskScheduledEvent(
	version int64,
	scheduledEventID int64,
	taskQueue *taskqueuepb.TaskQueue,
	startToCloseTimeout *time.Duration,
	attempt int32,
	scheduledTime *time.Time,
	originalScheduledTimestamp *time.Time,
) (*WorkflowTaskInfo, error) {

	// set workflow state to running, since workflow task is scheduled
	// NOTE: for zombie workflow, should not change the state
	state, _ := m.ms.GetWorkflowStateStatus()
	if state != enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE {
		if err := m.ms.UpdateWorkflowStateStatus(
			enumsspb.WORKFLOW_EXECUTION_STATE_RUNNING,
			enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
		); err != nil {
			return nil, err
		}
	}

	workflowTask := &WorkflowTaskInfo{
		Version:               version,
		ScheduledEventID:      scheduledEventID,
		StartedEventID:        common.EmptyEventID,
		RequestID:             emptyUUID,
		WorkflowTaskTimeout:   startToCloseTimeout,
		TaskQueue:             taskQueue,
		Attempt:               attempt,
		ScheduledTime:         scheduledTime,
		StartedTime:           nil,
		OriginalScheduledTime: originalScheduledTimestamp,
	}

	m.UpdateWorkflowTask(workflowTask)
	return workflowTask, nil
}

func (m *workflowTaskStateMachine) ReplicateTransientWorkflowTaskScheduled() (*WorkflowTaskInfo, error) {
	// When workflow task fails/timeout it gets removed from mutable state, but attempt count is incremented.
	// If attempt count was incremented and now is greater than 1, then next workflow task should be created as transient.
	// This method will do it only if there is no other workflow task and next workflow task should be transient.
	if m.HasPendingWorkflowTask() || !m.ms.IsTransientWorkflowTask() {
		return nil, nil
	}

	// Because this method is called from ApplyEvents, where nextEventID is updated at the very end of the func,
	// ScheduledEventID for this workflow task is guaranteed to be wrong.
	// But this is OK. Whatever ScheduledEventID value is set here it will be stored in mutable state
	// and returned to the caller. It must be the same value because it will be used to look up
	// workflow task from mutable state.
	// There are 2 possible scenarios:
	//   1. If a failover happen just after this transient workflow task is scheduled with wrong ScheduledEventID,
	//   then AddWorkflowTaskStartedEvent will handle the correction of ScheduledEventID,
	//   and set the attempt to 1 (convert transient workflow task to normal workflow task).
	//   2. If no failover happen during the lifetime of this transient workflow task
	//   then ReplicateWorkflowTaskScheduledEvent will overwrite everything
	//   including the workflow task ScheduledEventID.
	//
	// Regarding workflow task timeout calculation:
	//   1. Attempt will be set to 1, so we still use default workflow task timeout.
	//   2. ReplicateWorkflowTaskScheduledEvent will overwrite everything including WorkflowTaskTimeout.
	workflowTask := &WorkflowTaskInfo{
		Version:             m.ms.GetCurrentVersion(),
		ScheduledEventID:    m.ms.GetNextEventID(),
		StartedEventID:      common.EmptyEventID,
		RequestID:           emptyUUID,
		WorkflowTaskTimeout: m.ms.GetExecutionInfo().DefaultWorkflowTaskTimeout,
		// Task queue is always of kind NORMAL because transient workflow task is created only for
		// failed/timed out workflow task and fail/timeout clears stickiness.
		TaskQueue:     m.ms.TaskQueue(),
		Attempt:       m.ms.GetExecutionInfo().WorkflowTaskAttempt,
		ScheduledTime: timestamp.TimePtr(m.ms.timeSource.Now()),
		StartedTime:   timestamp.UnixOrZeroTimePtr(0),
	}

	m.UpdateWorkflowTask(workflowTask)
	return workflowTask, nil
}

func (m *workflowTaskStateMachine) ReplicateWorkflowTaskStartedEvent(
	workflowTask *WorkflowTaskInfo,
	version int64,
	scheduledEventID int64,
	startedEventID int64,
	requestID string,
	timestamp time.Time,
) (*WorkflowTaskInfo, error) {
	// When this function is called from ApplyEvents, workflowTask is nil.
	// It is safe to look up the workflow task as it does not have to deal with transient workflow task case.
	if workflowTask == nil {
		var ok bool
		workflowTask, ok = m.GetWorkflowTaskInfo(scheduledEventID)
		if !ok {
			return nil, serviceerror.NewInternal(fmt.Sprintf("unable to find workflow task: %v", scheduledEventID))
		}
		// Transient workflow task events are not replicated but attempt count in mutable state
		// can be updated from previous workflow task failed/timeout event.
		// During replication, "active" side will send 2 batches:
		//   1. WorkflowTaskScheduledEvent and WorkflowTaskStartedEvent
		//   2. WorkflowTaskCompletedEvents & other events
		// Each batch is like a transaction but there is no guarantee that all batches will be delivered before failover is happened.
		// Therefore, we need to support case when 1st batch is replicated but 2nd is not.
		// If previous workflow task was failed/timed out, mutable state will have attempt count > 1.
		// Because attempt count controls whether workflow task is transient or not,
		// "standby" side would consider this workflow task as transient, while it is actually not.
		// To prevent this attempt count is reset to 1, workflow task will be treated as normal workflow task,
		// and will be timed out or completed correctly.
		workflowTask.Attempt = 1
	}

	workflowTask = &WorkflowTaskInfo{
		Version:               version,
		ScheduledEventID:      scheduledEventID,
		StartedEventID:        startedEventID,
		RequestID:             requestID,
		WorkflowTaskTimeout:   workflowTask.WorkflowTaskTimeout,
		Attempt:               workflowTask.Attempt,
		StartedTime:           &timestamp,
		ScheduledTime:         workflowTask.ScheduledTime,
		TaskQueue:             workflowTask.TaskQueue,
		OriginalScheduledTime: workflowTask.OriginalScheduledTime,
	}

	m.UpdateWorkflowTask(workflowTask)
	return workflowTask, nil
}

func (m *workflowTaskStateMachine) ReplicateWorkflowTaskCompletedEvent(
	event *historypb.HistoryEvent,
) error {
	m.beforeAddWorkflowTaskCompletedEvent()
	return m.afterAddWorkflowTaskCompletedEvent(event, math.MaxInt32)
}

func (m *workflowTaskStateMachine) ReplicateWorkflowTaskFailedEvent() error {
	m.FailWorkflowTask(true)
	return nil
}

func (m *workflowTaskStateMachine) ReplicateWorkflowTaskTimedOutEvent(
	timeoutType enumspb.TimeoutType,
) error {
	incrementAttempt := true
	// Do not increment workflow task attempt in the case of sticky timeout to prevent creating next workflow task as transient.
	if timeoutType == enumspb.TIMEOUT_TYPE_SCHEDULE_TO_START {
		incrementAttempt = false
	}
	m.FailWorkflowTask(incrementAttempt)
	return nil
}

func (m *workflowTaskStateMachine) AddWorkflowTaskScheduleToStartTimeoutEvent(
	scheduledEventID int64,
) (*historypb.HistoryEvent, error) {
	opTag := tag.WorkflowActionWorkflowTaskTimedOut
	if m.ms.executionInfo.WorkflowTaskScheduledEventId != scheduledEventID || m.ms.executionInfo.WorkflowTaskStartedEventId > 0 {
		m.ms.logger.Warn(mutableStateInvalidHistoryActionMsg, opTag,
			tag.WorkflowEventID(m.ms.GetNextEventID()),
			tag.ErrorTypeInvalidHistoryAction,
			tag.WorkflowScheduledEventID(scheduledEventID),
		)
		return nil, m.ms.createInternalServerError(opTag)
	}

	// clear stickiness whenever workflow task fails
	m.ms.ClearStickyness()

	event := m.ms.hBuilder.AddWorkflowTaskTimedOutEvent(
		scheduledEventID,
		common.EmptyEventID,
		enumspb.TIMEOUT_TYPE_SCHEDULE_TO_START,
	)
	if err := m.ReplicateWorkflowTaskTimedOutEvent(enumspb.TIMEOUT_TYPE_SCHEDULE_TO_START); err != nil {
		return nil, err
	}
	return event, nil
}

// AddWorkflowTaskScheduledEventAsHeartbeat is to record the first scheduled workflow task during workflow task heartbeat.
func (m *workflowTaskStateMachine) AddWorkflowTaskScheduledEventAsHeartbeat(
	bypassTaskGeneration bool,
	originalScheduledTimestamp *time.Time,
) (*WorkflowTaskInfo, error) {
	opTag := tag.WorkflowActionWorkflowTaskScheduled
	if m.HasPendingWorkflowTask() {
		m.ms.logger.Warn(mutableStateInvalidHistoryActionMsg, opTag,
			tag.WorkflowEventID(m.ms.GetNextEventID()),
			tag.ErrorTypeInvalidHistoryAction,
			tag.WorkflowScheduledEventID(m.ms.executionInfo.WorkflowTaskScheduledEventId))
		return nil, m.ms.createInternalServerError(opTag)
	}

	// Create real WorkflowTaskScheduledEvent when workflow task:
	//  - is not transient (is not continuously failing)
	//  and
	//  - is not speculative.
	createWorkflowTaskScheduledEvent := !m.ms.IsTransientWorkflowTask()

	// If while scheduling a workflow task and new events has come, then this workflow task cannot be a transient/speculative.
	// Flush any buffered events before creating the workflow task, otherwise it will result in invalid IDs for
	// transient/speculative workflow task and will cause in timeout processing to not work for transient workflow tasks.
	if m.ms.HasBufferedEvents() {
		m.ms.executionInfo.WorkflowTaskAttempt = 1
		createWorkflowTaskScheduledEvent = true
		m.ms.updatePendingEventIDs(m.ms.hBuilder.FlushBufferToCurrentBatch())
	}
	if !createWorkflowTaskScheduledEvent {
		lastWriteVersion, err := m.ms.GetLastWriteVersion()
		if err != nil {
			return nil, err
		}

		// If failover happened during transient workflow task,
		// then reset the attempt to 1, and not use transient workflow task.
		if m.ms.GetCurrentVersion() != lastWriteVersion {
			m.ms.executionInfo.WorkflowTaskAttempt = 1
			createWorkflowTaskScheduledEvent = true
		}
	}

	scheduleTime := m.ms.timeSource.Now().UTC()
	attempt := m.ms.executionInfo.WorkflowTaskAttempt
	// TaskQueue should already be set from workflow execution started event.
	taskQueue := m.ms.TaskQueue()
	// DefaultWorkflowTaskTimeout should already be set from workflow execution started event.
	startToCloseTimeout := m.getStartToCloseTimeout(m.ms.executionInfo.DefaultWorkflowTaskTimeout, attempt)

	var scheduledEvent *historypb.HistoryEvent
	var scheduledEventID int64

	if createWorkflowTaskScheduledEvent {
		scheduledEvent = m.ms.hBuilder.AddWorkflowTaskScheduledEvent(
			taskQueue,
			startToCloseTimeout,
			attempt,
			scheduleTime,
		)
		scheduledEventID = scheduledEvent.GetEventId()
	} else {
		// WorkflowTaskScheduledEvent will be created later.
		scheduledEventID = m.ms.GetNextEventID()
	}

	workflowTask, err := m.ReplicateWorkflowTaskScheduledEvent(
		m.ms.GetCurrentVersion(),
		scheduledEventID,
		taskQueue,
		startToCloseTimeout,
		attempt,
		&scheduleTime,
		originalScheduledTimestamp,
	)
	if err != nil {
		return nil, err
	}

	// TODO merge active & passive task generation
	if !bypassTaskGeneration {
		if err := m.ms.taskGenerator.GenerateScheduleWorkflowTaskTasks(
			scheduledEventID,
		); err != nil {
			return nil, err
		}
	}

	return workflowTask, nil
}

func (m *workflowTaskStateMachine) AddWorkflowTaskScheduledEvent(
	bypassTaskGeneration bool,
) (*WorkflowTaskInfo, error) {
	return m.AddWorkflowTaskScheduledEventAsHeartbeat(bypassTaskGeneration, timestamp.TimePtr(m.ms.timeSource.Now()))
}

func (m *workflowTaskStateMachine) AddFirstWorkflowTaskScheduled(
	startEvent *historypb.HistoryEvent,
) error {
	// handle first workflow task case, i.e. possible delayed workflow task
	//
	// below handles the following cases:
	// 1. if not continue as new & if workflow has no parent
	//   -> schedule workflow task & schedule delayed workflow task
	// 2. if not continue as new & if workflow has parent
	//   -> this function should not be called during workflow start, but should be called as
	//      part of schedule workflow task in 2 phase commit
	//
	// if continue as new
	//  1. whether has parent workflow or not
	//   -> schedule workflow task & schedule delayed workflow task
	//
	startAttr := startEvent.GetWorkflowExecutionStartedEventAttributes()
	workflowTaskBackoffDuration := timestamp.DurationValue(startAttr.GetFirstWorkflowTaskBackoff())

	var err error
	if workflowTaskBackoffDuration != 0 {
		if err = m.ms.taskGenerator.GenerateDelayedWorkflowTasks(
			startEvent,
		); err != nil {
			return err
		}
	} else {
		if _, err = m.AddWorkflowTaskScheduledEvent(
			false,
		); err != nil {
			return err
		}
	}

	return nil
}

func (m *workflowTaskStateMachine) AddWorkflowTaskStartedEvent(
	scheduledEventID int64,
	requestID string,
	taskQueue *taskqueuepb.TaskQueue,
	identity string,
) (*historypb.HistoryEvent, *WorkflowTaskInfo, error) {
	opTag := tag.WorkflowActionWorkflowTaskStarted
	workflowTask, ok := m.GetWorkflowTaskInfo(scheduledEventID)
	if !ok || workflowTask.StartedEventID != common.EmptyEventID {
		m.ms.logger.Warn(mutableStateInvalidHistoryActionMsg, opTag,
			tag.WorkflowEventID(m.ms.GetNextEventID()),
			tag.ErrorTypeInvalidHistoryAction,
			tag.WorkflowScheduledEventID(scheduledEventID))
		return nil, nil, m.ms.createInternalServerError(opTag)
	}

	scheduledEventID = workflowTask.ScheduledEventID
	startedEventID := scheduledEventID + 1
	startTime := m.ms.timeSource.Now()
	workflowTaskScheduledEventCreated := !m.ms.IsTransientWorkflowTask()

	// If new events came since transient/speculative workflow task was scheduled
	// or failover happened during lifetime of transient workflow task,
	// transient/speculative workflow task needs to be converted to normal workflow task,
	// i.e. WorkflowTaskScheduledEvent needs to be created now.
	if !workflowTaskScheduledEventCreated &&
		(workflowTask.ScheduledEventID != m.ms.GetNextEventID() || workflowTask.Version != m.ms.GetCurrentVersion()) {

		workflowTask.Attempt = 1
		workflowTaskScheduledEventCreated = true
		scheduledEvent := m.ms.hBuilder.AddWorkflowTaskScheduledEvent(
			taskQueue,
			workflowTask.WorkflowTaskTimeout,
			workflowTask.Attempt,
			startTime,
		)
		scheduledEventID = scheduledEvent.GetEventId()
	}

	// Create WorkflowTaskStartedEvent only if WorkflowTaskScheduledEvent was created.
	// (it wasn't created for transient/speculative workflow task).
	var startedEvent *historypb.HistoryEvent
	if workflowTaskScheduledEventCreated {
		startedEvent = m.ms.hBuilder.AddWorkflowTaskStartedEvent(
			scheduledEventID,
			requestID,
			identity,
			startTime,
		)
		m.ms.hBuilder.FlushAndCreateNewBatch()
		startedEventID = startedEvent.GetEventId()
	}

	workflowTask, err := m.ReplicateWorkflowTaskStartedEvent(workflowTask, m.ms.GetCurrentVersion(), scheduledEventID, startedEventID, requestID, startTime)

	m.emitWorkflowTaskAttemptStats(workflowTask.Attempt)

	// TODO merge active & passive task generation
	if err := m.ms.taskGenerator.GenerateStartWorkflowTaskTasks(
		scheduledEventID,
	); err != nil {
		return nil, nil, err
	}
	return startedEvent, workflowTask, err
}

func (m *workflowTaskStateMachine) AddWorkflowTaskCompletedEvent(
	scheduledEventID int64,
	startedEventID int64,
	request *workflowservice.RespondWorkflowTaskCompletedRequest,
	maxResetPoints int,
) (*historypb.HistoryEvent, error) {
	opTag := tag.WorkflowActionWorkflowTaskCompleted
	workflowTask, ok := m.GetWorkflowTaskInfo(scheduledEventID)
	if !ok || workflowTask.StartedEventID != startedEventID {
		m.ms.logger.Warn(mutableStateInvalidHistoryActionMsg, opTag,
			tag.WorkflowEventID(m.ms.GetNextEventID()),
			tag.ErrorTypeInvalidHistoryAction,
			tag.WorkflowScheduledEventID(scheduledEventID),
			tag.WorkflowStartedEventID(startedEventID))

		return nil, m.ms.createInternalServerError(opTag)
	}

	// Capture if WorkflowTaskScheduled and WorkflowTaskStarted events were created
	// before calling m.beforeAddWorkflowTaskCompletedEvent() because it will delete workflow task info from mutable state.
	workflowTaskScheduledStartedEventsCreated := !m.ms.IsTransientWorkflowTask()
	m.beforeAddWorkflowTaskCompletedEvent()

	if !workflowTaskScheduledStartedEventsCreated {
		// Create corresponding WorkflowTaskScheduled and WorkflowTaskStarted events for transient/speculative workflow tasks.
		scheduledEvent := m.ms.hBuilder.AddWorkflowTaskScheduledEvent(
			m.ms.TaskQueue(),
			workflowTask.WorkflowTaskTimeout,
			workflowTask.Attempt,
			timestamp.TimeValue(workflowTask.ScheduledTime).UTC(),
		)
		startedEvent := m.ms.hBuilder.AddWorkflowTaskStartedEvent(
			scheduledEvent.GetEventId(),
			workflowTask.RequestID,
			request.GetIdentity(),
			timestamp.TimeValue(workflowTask.StartedTime),
		)
		m.ms.hBuilder.FlushAndCreateNewBatch()
		startedEventID = startedEvent.GetEventId()
	}
	// Now write the completed event
	event := m.ms.hBuilder.AddWorkflowTaskCompletedEvent(
		scheduledEventID,
		startedEventID,
		request.Identity,
		request.BinaryChecksum,
	)

	err := m.afterAddWorkflowTaskCompletedEvent(event, maxResetPoints)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (m *workflowTaskStateMachine) AddWorkflowTaskFailedEvent(
	scheduledEventID int64,
	startedEventID int64,
	cause enumspb.WorkflowTaskFailedCause,
	failure *failurepb.Failure,
	identity string,
	binChecksum string,
	baseRunID string,
	newRunID string,
	forkEventVersion int64,
) (*historypb.HistoryEvent, error) {
	opTag := tag.WorkflowActionWorkflowTaskFailed
	attr := &historypb.WorkflowTaskFailedEventAttributes{
		ScheduledEventId: scheduledEventID,
		StartedEventId:   startedEventID,
		Cause:            cause,
		Failure:          failure,
		Identity:         identity,
		BinaryChecksum:   binChecksum,
		BaseRunId:        baseRunID,
		NewRunId:         newRunID,
		ForkEventVersion: forkEventVersion,
	}

	workflowTask, ok := m.GetWorkflowTaskInfo(scheduledEventID)
	if !ok || workflowTask.StartedEventID != startedEventID {
		m.ms.logger.Warn(mutableStateInvalidHistoryActionMsg, opTag,
			tag.WorkflowEventID(m.ms.GetNextEventID()),
			tag.ErrorTypeInvalidHistoryAction,
			tag.WorkflowScheduledEventID(scheduledEventID),
			tag.WorkflowStartedEventID(startedEventID))
		return nil, m.ms.createInternalServerError(opTag)
	}

	var event *historypb.HistoryEvent
	// Only emit WorkflowTaskFailedEvent if workflow task is not transient.
	if !m.ms.IsTransientWorkflowTask() {
		event = m.ms.hBuilder.AddWorkflowTaskFailedEvent(
			attr.ScheduledEventId,
			attr.StartedEventId,
			attr.Cause,
			attr.Failure,
			attr.Identity,
			attr.BaseRunId,
			attr.NewRunId,
			attr.ForkEventVersion,
			attr.BinaryChecksum,
		)
	}

	if err := m.ReplicateWorkflowTaskFailedEvent(); err != nil {
		return nil, err
	}

	// always clear workflow task attempt for reset
	if cause == enumspb.WORKFLOW_TASK_FAILED_CAUSE_RESET_WORKFLOW ||
		cause == enumspb.WORKFLOW_TASK_FAILED_CAUSE_FAILOVER_CLOSE_COMMAND {
		m.ms.executionInfo.WorkflowTaskAttempt = 1
	}
	return event, nil
}

func (m *workflowTaskStateMachine) AddWorkflowTaskTimedOutEvent(
	scheduledEventID int64,
	startedEventID int64,
) (*historypb.HistoryEvent, error) {
	opTag := tag.WorkflowActionWorkflowTaskTimedOut
	workflowTask, ok := m.GetWorkflowTaskInfo(scheduledEventID)
	if !ok || workflowTask.StartedEventID != startedEventID {
		m.ms.logger.Warn(mutableStateInvalidHistoryActionMsg, opTag,
			tag.WorkflowEventID(m.ms.GetNextEventID()),
			tag.ErrorTypeInvalidHistoryAction,
			tag.WorkflowScheduledEventID(scheduledEventID),
			tag.WorkflowStartedEventID(startedEventID))
		return nil, m.ms.createInternalServerError(opTag)
	}

	var event *historypb.HistoryEvent
	// Avoid creating WorkflowTaskTimedOut history event when workflow task is transient.
	if !m.ms.IsTransientWorkflowTask() {
		event = m.ms.hBuilder.AddWorkflowTaskTimedOutEvent(
			scheduledEventID,
			startedEventID,
			enumspb.TIMEOUT_TYPE_START_TO_CLOSE,
		)
	}

	if err := m.ReplicateWorkflowTaskTimedOutEvent(enumspb.TIMEOUT_TYPE_START_TO_CLOSE); err != nil {
		return nil, err
	}
	return event, nil
}

func (m *workflowTaskStateMachine) FailWorkflowTask(
	incrementAttempt bool,
) {
	// Increment attempts only if workflow task is failing on non-sticky task queue.
	// If it was stick task queue, clear stickiness first and try again before creating transient workflow tas.
	incrementAttempt = incrementAttempt && !m.ms.IsStickyTaskQueueEnabled()
	m.ms.ClearStickyness()

	failWorkflowTaskInfo := &WorkflowTaskInfo{
		Version:               common.EmptyVersion,
		ScheduledEventID:      common.EmptyEventID,
		StartedEventID:        common.EmptyEventID,
		RequestID:             emptyUUID,
		WorkflowTaskTimeout:   timestamp.DurationFromSeconds(0),
		StartedTime:           timestamp.UnixOrZeroTimePtr(0),
		TaskQueue:             nil,
		OriginalScheduledTime: timestamp.UnixOrZeroTimePtr(0),
		Attempt:               1,
	}
	if incrementAttempt {
		failWorkflowTaskInfo.Attempt = m.ms.executionInfo.WorkflowTaskAttempt + 1
		failWorkflowTaskInfo.ScheduledTime = timestamp.TimePtr(m.ms.timeSource.Now().UTC())
	}
	m.UpdateWorkflowTask(failWorkflowTaskInfo)
}

// DeleteWorkflowTask deletes a workflow task.
func (m *workflowTaskStateMachine) DeleteWorkflowTask() {
	resetWorkflowTaskInfo := &WorkflowTaskInfo{
		Version:             common.EmptyVersion,
		ScheduledEventID:    common.EmptyEventID,
		StartedEventID:      common.EmptyEventID,
		RequestID:           emptyUUID,
		WorkflowTaskTimeout: timestamp.DurationFromSeconds(0),
		Attempt:             1,
		StartedTime:         timestamp.UnixOrZeroTimePtr(0),
		ScheduledTime:       timestamp.UnixOrZeroTimePtr(0),

		TaskQueue: nil,
		// Keep the last original scheduled Timestamp, so that AddWorkflowTaskScheduledEventAsHeartbeat can continue with it.
		OriginalScheduledTime: m.getWorkflowTaskInfo().OriginalScheduledTime,
	}
	m.UpdateWorkflowTask(resetWorkflowTaskInfo)
}

// UpdateWorkflowTask updates a workflow task.
func (m *workflowTaskStateMachine) UpdateWorkflowTask(
	workflowTask *WorkflowTaskInfo,
) {
	m.ms.executionInfo.WorkflowTaskVersion = workflowTask.Version
	m.ms.executionInfo.WorkflowTaskScheduledEventId = workflowTask.ScheduledEventID
	m.ms.executionInfo.WorkflowTaskStartedEventId = workflowTask.StartedEventID
	m.ms.executionInfo.WorkflowTaskRequestId = workflowTask.RequestID
	m.ms.executionInfo.WorkflowTaskTimeout = workflowTask.WorkflowTaskTimeout
	m.ms.executionInfo.WorkflowTaskAttempt = workflowTask.Attempt
	m.ms.executionInfo.WorkflowTaskStartedTime = workflowTask.StartedTime
	m.ms.executionInfo.WorkflowTaskScheduledTime = workflowTask.ScheduledTime
	m.ms.executionInfo.WorkflowTaskOriginalScheduledTime = workflowTask.OriginalScheduledTime

	// NOTE: do not update taskqueue in execution info

	m.ms.logger.Debug("Workflow task updated",
		tag.WorkflowScheduledEventID(workflowTask.ScheduledEventID),
		tag.WorkflowStartedEventID(workflowTask.StartedEventID),
		tag.WorkflowTaskRequestId(workflowTask.RequestID),
		tag.WorkflowTaskTimeout(workflowTask.WorkflowTaskTimeout),
		tag.Attempt(workflowTask.Attempt),
		tag.WorkflowStartedTimestamp(workflowTask.StartedTime))
}

func (m *workflowTaskStateMachine) HasPendingWorkflowTask() bool {
	return m.ms.executionInfo.WorkflowTaskScheduledEventId != common.EmptyEventID
}

func (m *workflowTaskStateMachine) GetPendingWorkflowTask() (*WorkflowTaskInfo, bool) {
	if m.ms.executionInfo.WorkflowTaskScheduledEventId == common.EmptyEventID {
		return nil, false
	}

	workflowTask := m.getWorkflowTaskInfo()
	return workflowTask, true
}

func (m *workflowTaskStateMachine) HasInFlightWorkflowTask() bool {
	return m.ms.executionInfo.WorkflowTaskStartedEventId != common.EmptyEventID
}

func (m *workflowTaskStateMachine) GetInFlightWorkflowTask() (*WorkflowTaskInfo, bool) {
	if m.ms.executionInfo.WorkflowTaskScheduledEventId == common.EmptyEventID ||
		m.ms.executionInfo.WorkflowTaskStartedEventId == common.EmptyEventID {
		return nil, false
	}

	workflowTask := m.getWorkflowTaskInfo()
	return workflowTask, true
}

func (m *workflowTaskStateMachine) HasProcessedOrPendingWorkflowTask() bool {
	return m.HasPendingWorkflowTask() || m.ms.GetPreviousStartedEventID() != common.EmptyEventID
}

// GetWorkflowTaskInfo returns details about the in-progress workflow task
func (m *workflowTaskStateMachine) GetWorkflowTaskInfo(
	scheduledEventID int64,
) (*WorkflowTaskInfo, bool) {
	workflowTask := m.getWorkflowTaskInfo()
	if scheduledEventID == workflowTask.ScheduledEventID {
		return workflowTask, true
	}
	return nil, false
}

func (m *workflowTaskStateMachine) GetTransientWorkflowTaskInfo(
	workflowTask *WorkflowTaskInfo,
	identity string,
) *historyspb.TransientWorkflowTaskInfo {

	// Create scheduled and started events which are not written to the history yet.
	scheduledEvent := &historypb.HistoryEvent{
		EventId:   workflowTask.ScheduledEventID,
		EventTime: workflowTask.ScheduledTime,
		EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED,
		Version:   m.ms.currentVersion,
		Attributes: &historypb.HistoryEvent_WorkflowTaskScheduledEventAttributes{
			WorkflowTaskScheduledEventAttributes: &historypb.WorkflowTaskScheduledEventAttributes{
				TaskQueue:           m.ms.TaskQueue(),
				StartToCloseTimeout: workflowTask.WorkflowTaskTimeout,
				Attempt:             workflowTask.Attempt,
			},
		},
	}

	startedEvent := &historypb.HistoryEvent{
		EventId:   workflowTask.StartedEventID,
		EventTime: workflowTask.StartedTime,
		EventType: enumspb.EVENT_TYPE_WORKFLOW_TASK_STARTED,
		Version:   m.ms.currentVersion,
		Attributes: &historypb.HistoryEvent_WorkflowTaskStartedEventAttributes{
			WorkflowTaskStartedEventAttributes: &historypb.WorkflowTaskStartedEventAttributes{
				ScheduledEventId: workflowTask.ScheduledEventID,
				Identity:         identity,
				RequestId:        workflowTask.RequestID,
			},
		},
	}

	return &historyspb.TransientWorkflowTaskInfo{
		HistorySuffix: []*historypb.HistoryEvent{scheduledEvent, startedEvent},
	}
}

func (m *workflowTaskStateMachine) getWorkflowTaskInfo() *WorkflowTaskInfo {
	return &WorkflowTaskInfo{
		Version:               m.ms.executionInfo.WorkflowTaskVersion,
		ScheduledEventID:      m.ms.executionInfo.WorkflowTaskScheduledEventId,
		StartedEventID:        m.ms.executionInfo.WorkflowTaskStartedEventId,
		RequestID:             m.ms.executionInfo.WorkflowTaskRequestId,
		WorkflowTaskTimeout:   m.ms.executionInfo.WorkflowTaskTimeout,
		Attempt:               m.ms.executionInfo.WorkflowTaskAttempt,
		StartedTime:           m.ms.executionInfo.WorkflowTaskStartedTime,
		ScheduledTime:         m.ms.executionInfo.WorkflowTaskScheduledTime,
		TaskQueue:             m.ms.TaskQueue(),
		OriginalScheduledTime: m.ms.executionInfo.WorkflowTaskOriginalScheduledTime,
	}
}

func (m *workflowTaskStateMachine) beforeAddWorkflowTaskCompletedEvent() {
	// Make sure to delete workflow task before adding events. Otherwise they are buffered rather than getting appended.
	m.DeleteWorkflowTask()
}

func (m *workflowTaskStateMachine) afterAddWorkflowTaskCompletedEvent(
	event *historypb.HistoryEvent,
	maxResetPoints int,
) error {
	m.ms.executionInfo.LastWorkflowTaskStartedEventId = event.GetWorkflowTaskCompletedEventAttributes().GetStartedEventId()
	return m.ms.addBinaryCheckSumIfNotExists(event, maxResetPoints)
}

func (m *workflowTaskStateMachine) emitWorkflowTaskAttemptStats(
	attempt int32,
) {
	namespaceName := m.ms.GetNamespaceEntry().Name().String()
	m.ms.metricsHandler.Histogram(metrics.WorkflowTaskAttempt.GetMetricName(), metrics.WorkflowTaskAttempt.GetMetricUnit()).
		Record(int64(attempt), metrics.NamespaceTag(namespaceName))
	if attempt >= int32(m.ms.shard.GetConfig().WorkflowTaskCriticalAttempts()) {
		m.ms.shard.GetThrottledLogger().Warn("Critical attempts processing workflow task",
			tag.WorkflowNamespace(namespaceName),
			tag.WorkflowID(m.ms.GetExecutionInfo().WorkflowId),
			tag.WorkflowRunID(m.ms.GetExecutionState().RunId),
			tag.Attempt(attempt),
		)
	}
}

func (m *workflowTaskStateMachine) getStartToCloseTimeout(
	defaultTimeout *time.Duration,
	attempt int32,
) *time.Duration {
	// This util function is only for calculating active workflow task timeout.
	// Transient workflow task in passive cluster won't call this function and
	// always use default timeout as it will either be completely overwritten by
	// a replicated workflow schedule event from active cluster, or if used, it's
	// attempt will be reset to 1.
	// Check ReplicateTransientWorkflowTaskScheduled for details.

	if defaultTimeout == nil {
		defaultTimeout = timestamp.DurationPtr(0)
	}

	if attempt <= workflowTaskRetryBackoffMinAttempts {
		return defaultTimeout
	}

	policy := backoff.NewExponentialRetryPolicy(workflowTaskRetryInitialInterval).
		WithMaximumInterval(m.ms.shard.GetConfig().WorkflowTaskRetryMaxInterval()).
		WithExpirationInterval(backoff.NoInterval)
	startToCloseTimeout := *defaultTimeout + policy.ComputeNextDelay(0, int(attempt)-workflowTaskRetryBackoffMinAttempts)
	return &startToCloseTimeout
}
