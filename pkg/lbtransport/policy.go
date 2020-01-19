package lbtransport

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// TargetPicker decides which target to pick for a given call.
type TargetPicker interface {
	// Pick decides on which target to use for the request out of the provided ones.
	Pick(targets []*Target) *Target
	// ExcludeHost excludes the given target for a short time. It is useful to report no connection (even temporary).
	ExcludeTarget(*Target)
}

// Target represents the canonical address of a backend.
type Target struct {
	DialAddr string
}

// RoundRobinPicker picks target using round robin behaviour.
// It does NOT dial to the chosen target to check if it is accessible, instead it exposes ExcludeTarget method that allows to report
// connection troubles. That handles the situation when DNS resolution contains invalid targets. In that case, it
// blacklists it for defined period of time called "blacklist backoff".
type RoundRobinPicker struct {
	blacklistBackoffDuration time.Duration
	blacklistMu              sync.RWMutex
	blacklistedTargets       map[Target]time.Time

	roundRobinCounter uint64

	// For testing purposes.
	timeNow func() time.Time
}

func NewRoundRobinPicker(ctx context.Context, backoffDuration time.Duration) *RoundRobinPicker {
	rr := &RoundRobinPicker{
		blacklistBackoffDuration: backoffDuration,
		blacklistedTargets:       make(map[Target]time.Time),
		timeNow:                  time.Now,
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Minute):
			}
			rr.cleanUpBlacklist()
		}
	}()

	return rr
}

func (rr *RoundRobinPicker) cleanUpBlacklist() {
	rr.blacklistMu.Lock()
	defer rr.blacklistMu.Unlock()

	for target, failTime := range rr.blacklistedTargets {
		if failTime.Add(rr.blacklistBackoffDuration).Before(rr.timeNow()) {
			delete(rr.blacklistedTargets, target) // Expired.
		}
	}
}

func (rr *RoundRobinPicker) isTargetBlacklisted(target *Target) bool {
	rr.blacklistMu.RLock()
	failTime, ok := rr.blacklistedTargets[*target]
	rr.blacklistMu.RUnlock()

	if !ok {
		return false
	}

	// It is blacklisted, but check if still valid. If not then false - it's not actually blacklisted.
	return failTime.Add(rr.blacklistBackoffDuration).After(rr.timeNow())
}

func (rr *RoundRobinPicker) Pick(targets []*Target) *Target {
	for range targets {
		id := atomic.AddUint64(&(rr.roundRobinCounter), 1)
		targetID := int(id % uint64(len(targets)))
		target := targets[targetID]

		if rr.isTargetBlacklisted(target) {
			// That target is blacklisted. Check another one.
			continue
		}

		return target
	}

	return nil
}

func (rr *RoundRobinPicker) ExcludeTarget(target *Target) {
	rr.blacklistMu.Lock()
	defer rr.blacklistMu.Unlock()

	rr.blacklistedTargets[*target] = rr.timeNow()
}
