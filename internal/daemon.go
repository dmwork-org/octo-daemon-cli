package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Daemon struct {
	cfg      Config
	client   *Client
	daemonID string
	lockFile *os.File

	mu                 sync.Mutex
	registeredRuntimes []RegisteredRuntime
	lastRuntimes       []RuntimeInfo
	generation         uint64 // incremented on every successful doRegister
	heartbeatCount     int

	slowDetectRunning atomic.Bool // single-flight guard for slow OpenClaw detection
}

func NewDaemon(cfg Config) (*Daemon, error) {
	cfg.withDefaults()

	daemonID, err := EnsureDaemonID()
	if err != nil {
		return nil, fmt.Errorf("ensure daemon id: %w", err)
	}

	client := NewClient(cfg.APIURL, cfg.APIKey, cfg.CLIVersion)

	return &Daemon{
		cfg:      cfg,
		client:   client,
		daemonID: daemonID,
	}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	lockFile, err := TryLock()
	if err != nil {
		return err
	}
	d.lockFile = lockFile
	defer func() {
		RemovePID()
		d.lockFile.Close()
		os.Remove(LockFilePath())
	}()

	log.Printf("[INFO] daemon starting (id=%s, device=%s)", d.daemonID, d.cfg.DeviceName)

	if err := d.register(ctx); err != nil {
		return fmt.Errorf("initial registration: %w", err)
	}

	defer d.deregister()

	return d.heartbeatLoop(ctx)
}

func (d *Daemon) addDeviceName(runtimes []RuntimeInfo) {
	for i := range runtimes {
		runtimes[i].Name = fmt.Sprintf("%s (%s)", capitalize(runtimes[i].Provider), d.cfg.DeviceName)
	}
}

func (d *Daemon) register(ctx context.Context) error {
	runtimes := DetectRuntimesFast()
	d.addDeviceName(runtimes)

	if len(runtimes) == 0 {
		log.Printf("[WARN] no agent runtimes detected on this machine")
	}

	for _, r := range runtimes {
		log.Printf("[INFO] detected: %s %s (%s)", r.Provider, r.Version, r.Path)
		if r.Status != "online" {
			log.Printf("[INFO]   %s status: %s (will skip heartbeat)", r.Provider, r.Status)
		}
	}

	resp, err := d.client.Register(ctx, d.buildRegisterRequest(runtimes))
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.generation++
	d.lastRuntimes = runtimes
	d.registeredRuntimes = resp.Runtimes
	gen := d.generation
	d.mu.Unlock()
	log.Printf("[INFO] registered %d runtime(s) with server (gen=%d)", len(resp.Runtimes), gen)

	// Async enrich: slow OpenClaw agents detection
	d.startSlowDetect(ctx, gen)
	return nil
}

func (d *Daemon) heartbeatLoop(ctx context.Context) error {
	ticker := time.NewTicker(d.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			d.heartbeatCount++
			d.sendHeartbeats(ctx)

			if d.heartbeatCount%4 == 0 {
				d.mu.Lock()
				gen := d.generation
				d.mu.Unlock()
				d.startSlowDetect(ctx, gen)
			}
		}
	}
}

// startSlowDetect runs full detection (including slow OpenClaw agents list) in a
// single-flight goroutine. Only one slow detect runs at a time. When it completes,
// it only registers if the generation hasn't advanced (no newer state was committed).
func (d *Daemon) startSlowDetect(ctx context.Context, startGen uint64) {
	if !d.slowDetectRunning.CompareAndSwap(false, true) {
		return // another slow detect is already running
	}

	go func() {
		defer d.slowDetectRunning.Store(false)

		current := DetectRuntimesFast()
		d.addDeviceName(current)
		current = EnrichOpenclawAgents(current)

		d.mu.Lock()
		if d.generation != startGen {
			// State advanced while we were detecting; discard stale result
			d.mu.Unlock()
			log.Printf("[DEBUG] slow detect discarded (gen %d → %d)", startGen, d.generation)
			return
		}
		changed := runtimesChanged(d.lastRuntimes, current)
		d.mu.Unlock()

		if changed {
			log.Printf("[INFO] runtime changes detected, re-registering...")
			d.doRegister(ctx, current, startGen)
		}
	}()
}

// doRegister sends runtimes to server. Only commits state if generation matches
// expectedGen (prevents stale async results from overwriting newer state).
func (d *Daemon) doRegister(ctx context.Context, runtimes []RuntimeInfo, expectedGen uint64) {
	resp, err := d.client.Register(ctx, d.buildRegisterRequest(runtimes))
	if err != nil {
		log.Printf("[WARN] register failed: %v", err)
		return
	}

	d.mu.Lock()
	if d.generation != expectedGen {
		d.mu.Unlock()
		log.Printf("[DEBUG] register result discarded (gen %d → %d)", expectedGen, d.generation)
		return
	}
	d.generation++
	d.lastRuntimes = runtimes
	d.registeredRuntimes = resp.Runtimes
	d.mu.Unlock()
	log.Printf("[INFO] registered %d runtime(s) (gen=%d)", len(resp.Runtimes), expectedGen+1)
}

func (d *Daemon) buildRegisterRequest(runtimes []RuntimeInfo) RegisterRequest {
	return RegisterRequest{
		DaemonID:   d.daemonID,
		DeviceName: d.cfg.DeviceName,
		DeviceInfo: GetDeviceInfo(),
		CLIVersion: d.cfg.CLIVersion,
		Runtimes:   runtimes,
	}
}

func (d *Daemon) sendHeartbeats(ctx context.Context) {
	d.mu.Lock()
	offlineProviders := make(map[string]bool)
	for _, r := range d.lastRuntimes {
		if r.Status == "offline" {
			offlineProviders[r.Provider] = true
		}
	}
	registered := make([]RegisteredRuntime, len(d.registeredRuntimes))
	copy(registered, d.registeredRuntimes)
	gen := d.generation
	d.mu.Unlock()

	needReRegister := false
	for _, rt := range registered {
		if offlineProviders[rt.Provider] {
			continue
		}
		if err := d.client.Heartbeat(ctx, rt.ID); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[WARN] heartbeat failed for runtime %d (%s): %v", rt.ID, rt.Provider, err)
			needReRegister = true
		}
	}

	if needReRegister {
		log.Printf("[INFO] heartbeat failure detected, forcing re-register...")
		d.startSlowDetect(ctx, gen)
	}
}

func (d *Daemon) deregister() {
	d.mu.Lock()
	ids := make([]int64, len(d.registeredRuntimes))
	for i, rt := range d.registeredRuntimes {
		ids[i] = rt.ID
	}
	d.mu.Unlock()

	if len(ids) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.client.Deregister(ctx, ids); err != nil {
		log.Printf("[WARN] deregister failed: %v", err)
		return
	}

	log.Printf("[INFO] deregistered %d runtime(s)", len(ids))
}

func runtimesChanged(old, current []RuntimeInfo) bool {
	if len(old) != len(current) {
		return true
	}
	oldMap := make(map[string]RuntimeInfo)
	for _, r := range old {
		oldMap[r.Provider] = r
	}
	for _, r := range current {
		prev, ok := oldMap[r.Provider]
		if !ok || prev.Version != r.Version || prev.Status != r.Status || len(prev.Agents) != len(r.Agents) {
			return true
		}
		for i, a := range r.Agents {
			if i >= len(prev.Agents) || a.ID != prev.Agents[i].ID || a.Bindings != prev.Agents[i].Bindings {
				return true
			}
		}
	}
	return false
}

func agentIDs(agents []AgentEntry) string {
	ids := make([]string, len(agents))
	for i, a := range agents {
		ids[i] = a.ID
	}
	return fmt.Sprintf("[%s]", joinStrings(ids, ", "))
}

func joinStrings(s []string, sep string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += sep
		}
		result += v
	}
	return result
}
