# Debug Session: ci-node3-restart

Status: OPEN

## Symptom

- In CI, `TestFollowerRestartCatchesUpMissingWrites` appears to get stuck after restarting `node3`.
- The leader keeps logging Raft heartbeat / append failures to `node3` with `connect: connection refused`.
- Expected behavior: `node3` should restart, pass health check, rejoin replication, and catch up missing writes.

## Hypotheses

1. `node3` process exits during restart before HTTP and/or Raft listeners are fully bound.
2. `node3` restart fails because some local resource is not released in time in CI, such as BoltDB lock files or TCP ports.
3. `node3` stays alive but never becomes healthy because startup blocks on discovery / join sequencing.
4. `node3` restarts its HTTP server but Raft transport does not come up, so leader heartbeats keep failing.
5. The test harness misses a startup error from the restarted app instance, so the failure is only visible indirectly from leader logs.

## Plan

1. Add instrumentation around test node startup, shutdown, and async error reporting.
2. Add instrumentation around app startup milestones and listener bring-up.
3. Reproduce the restart scenario locally under `go test ./test/e2e -run TestFollowerRestartCatchesUpMissingWrites -v`.
4. Use evidence to confirm or reject the hypotheses.
5. Apply the minimal fix only after the evidence is clear.

## Evidence

- Pre-fix reproduction showed `node3` restart spending about 29s inside `raftnode.New()` while reopening `raft-log.db`.
- Pre-fix log sequence:
  - `internal/raftnode/node.go:New:log-store-begin` for restarted `node3` at `ts=1775802099328`
  - `internal/raftnode/node.go:New:log-store-ok` for restarted `node3` at `ts=1775802128508`
  - This gap is about `29.18s`.
- During that gap, leader logs continuously showed `connect refused` heartbeats to `node3`, which matches the CI symptom.
- `storage.Open` and `App.Run().JoinCluster()` were both fast, so they were ruled out as the blocking point.

## Root Cause

- `raftnode.New()` opened Raft BoltDB resources (`raft-log.db`, `raft-stable.db`) and the TCP transport.
- `Node.Close()` only called `raft.Shutdown()` and did not explicitly close the Bolt stores or the transport.
- Because the previous `node3` instance left those handles open until later cleanup, the restarted instance blocked waiting for the Raft log file lock to be released.

## Fix

- Persist the opened Raft resources on `Node`:
  - `logStore`
  - `stableStore`
  - `transport`
- Explicitly close all three in `Node.Close()` after `raft.Shutdown()`.

## Post-Fix Validation

- Post-fix reproduction:
  - restarted `node3` `log-store-begin` at `ts=1775802261501`
  - restarted `node3` `log-store-ok` at `ts=1775802261509`
  - reopen delay dropped to about `8ms`
- `go test ./test/e2e -run TestFollowerRestartCatchesUpMissingWrites -v` passed in about `4.3s`.
- `go test ./test/e2e -v` passed in about `12.3s`.

## Hypothesis Status

1. `node3` process exits during restart before listeners bind. -> Rejected
2. local resource release / file lock delay blocks restart. -> Confirmed
3. startup blocks on discovery / join sequencing. -> Rejected
4. HTTP starts but Raft transport does not come up. -> Rejected as primary cause
5. test harness misses async startup error. -> Rejected
