package health

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-conductor/health"
	"github.com/ethereum-optimism/optimism/op-conductor/metrics"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-service/dial"
)

var (
	ErrSequencerNotHealthy      = errors.New("sequencer is not healthy")
	ErrSequencerConnectionDown  = errors.New("cannot connect to sequencer rpc endpoints")
	ErrSupervisorConnectionDown = errors.New("cannot connect to supervisor rpc endpoint")
)

// BootstrapHealthMonitor is a custom health monitor for bootstrap operations.
// It implements the HealthMonitor interface but skips peer stats checking.
type BootstrapHealthMonitor struct {
	metrics            metrics.Metricer
	log                log.Logger
	node               dial.RollupClientInterface
	healthUpdateCh     chan error
	cancel             context.CancelFunc
	rollupCfg          *rollup.Config
	timeProviderFn     func() uint64
	wg                 sync.WaitGroup
	interval           uint64
	safeInterval       uint64
	lastSeenUnsafeNum  uint64
	lastSeenUnsafeTime uint64
	unsafeInterval     uint64
	safeEnabled        bool
}

// NewBootstrapHealthMonitor creates a new bootstrap health monitor.
func NewBootstrapHealthMonitor(
	log log.Logger,
	metrics metrics.Metricer,
	interval, unsafeInterval, safeInterval uint64,
	safeEnabled bool,
	rollupCfg *rollup.Config,
	node dial.RollupClientInterface,
) health.HealthMonitor {
	return &BootstrapHealthMonitor{
		log:            log,
		metrics:        metrics,
		interval:       interval,
		healthUpdateCh: make(chan error),
		rollupCfg:      rollupCfg,
		unsafeInterval: unsafeInterval,
		safeEnabled:    safeEnabled,
		safeInterval:   safeInterval,
		timeProviderFn: currentTimeProvider,
		node:           node,
	}
}

// Start implements HealthMonitor.
func (hm *BootstrapHealthMonitor) Start(ctx context.Context) error {
	if hm.cancel != nil {
		return errors.New("health monitor already started")
	}

	ctx, cancel := context.WithCancel(ctx)
	hm.cancel = cancel
	hm.log.Info("starting bootstrap health monitor", "interval", hm.interval)

	hm.wg.Add(1)
	go hm.loop(ctx)

	return nil
}

// Stop implements HealthMonitor.
func (hm *BootstrapHealthMonitor) Stop() error {
	if hm.cancel == nil {
		return errors.New("health monitor not started")
	}

	hm.log.Info("stopping bootstrap health monitor")

	// Signal cancellation to loop
	hm.cancel()

	// Wait for loop to fully exit before closing the channel
	// This ensures no goroutine will try to send on the channel after it's closed
	hm.wg.Wait()

	// Now it's safe to close the channel - no more sends will happen
	close(hm.healthUpdateCh)

	// Clear cancel function
	hm.cancel = nil

	hm.log.Info("bootstrap health monitor stopped")
	return nil
}

// Subscribe implements HealthMonitor.
func (hm *BootstrapHealthMonitor) Subscribe() <-chan error {
	return hm.healthUpdateCh
}

func (hm *BootstrapHealthMonitor) loop(ctx context.Context) {
	defer hm.wg.Done()

	duration := time.Duration(hm.interval) * time.Second //nolint:gosec
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := hm.healthCheck(ctx)
			hm.metrics.RecordHealthCheck(err == nil, err)
			// Ensure that we exit cleanly if told to shutdown while still waiting to publish the health update
			select {
			case hm.healthUpdateCh <- err:
				continue
			case <-ctx.Done():
				return
			}
		}
	}
}

// healthCheck checks the health of the sequencer.
// Unlike the original implementation, this version:
// - Does NOT check peer stats
// - Only checks unsafe head lag and safe head progress
func (hm *BootstrapHealthMonitor) healthCheck(ctx context.Context) error {
	status, err := hm.node.SyncStatus(ctx)
	if err != nil {
		hm.log.Error("health monitor failed to get sync status", "err", err)
		return ErrSequencerConnectionDown
	}

	now := hm.timeProviderFn()

	if status.UnsafeL2.Number > hm.lastSeenUnsafeNum {
		hm.lastSeenUnsafeNum = status.UnsafeL2.Number
		hm.lastSeenUnsafeTime = now
	}

	curUnsafeTimeDiff := calculateTimeDiff(now, status.UnsafeL2.Time)
	if curUnsafeTimeDiff > hm.unsafeInterval {
		hm.log.Error(
			"unsafe head is falling behind the unsafe interval",
			"now", now,
			"unsafe_head_num", status.UnsafeL2.Number,
			"unsafe_head_time", status.UnsafeL2.Time,
			"unsafe_interval", hm.unsafeInterval,
			"cur_unsafe_time_diff", curUnsafeTimeDiff,
		)
		return ErrSequencerNotHealthy
	}

	if hm.safeEnabled && calculateTimeDiff(now, status.SafeL2.Time) > hm.safeInterval {
		hm.log.Error(
			"safe head is not progressing as expected",
			"now", now,
			"safe_head_num", status.SafeL2.Number,
			"safe_head_time", status.SafeL2.Time,
			"safe_interval", hm.safeInterval,
		)
		return ErrSequencerNotHealthy
	}

	// NOTE: Peer stats checking is intentionally omitted in this implementation
	hm.log.Debug("bootstrap health check passed (peer stats check skipped)")

	hm.log.Info("sequencer is healthy")
	return nil
}

// currentTimeProvider returns the current time in Unix seconds.
func currentTimeProvider() uint64 {
	return uint64(time.Now().Unix()) //nolint:gosec
}

// calculateTimeDiff calculates the difference between two times in seconds.
func calculateTimeDiff(now, past uint64) uint64 {
	if now < past {
		return 0
	}
	return now - past
}
