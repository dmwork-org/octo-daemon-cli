package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

type Daemon struct {
	cfg      Config
	client   *Client
	daemonID string
	lockFile *os.File

	registeredRuntimes []RegisteredRuntime
	lastRuntimes       []RuntimeInfo
	heartbeatCount     int
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

func (d *Daemon) register(ctx context.Context) error {
	// Phase 1: fast detection (LookPath + version + gateway probe) — register immediately
	runtimes := DetectRuntimesFast()
	for i := range runtimes {
		runtimes[i].Name = fmt.Sprintf("%s (%s)", capitalize(runtimes[i].Provider), d.cfg.DeviceName)
	}

	if len(runtimes) == 0 {
		log.Printf("[WARN] no agent runtimes detected on this machine")
	}

	for _, r := range runtimes {
		log.Printf("[INFO] detected: %s %s (%s)", r.Provider, r.Version, r.Path)
		if r.Status != "online" {
			log.Printf("[INFO]   %s status: %s (will skip heartbeat)", r.Provider, r.Status)
		}
	}

	req := RegisterRequest{
		DaemonID:   d.daemonID,
		DeviceName: d.cfg.DeviceName,
		CLIVersion: d.cfg.CLIVersion,
		Runtimes:   runtimes,
	}

	resp, err := d.client.Register(ctx, req)
	if err != nil {
		return err
	}

	d.lastRuntimes = runtimes
	d.registeredRuntimes = resp.Runtimes
	log.Printf("[INFO] registered %d runtime(s) with server", len(d.registeredRuntimes))

	// Phase 2: slow enrichment (openclaw agents list) — async, then re-register
	go func() {
		enriched := EnrichOpenclawAgents(runtimes)
		if runtimesChanged(runtimes, enriched) {
			log.Printf("[INFO] enriched runtime details available, re-registering...")
			d.reRegister(ctx, enriched)
		}
	}()

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

			// Every 4 heartbeats (~60s), re-detect and re-register if changed
			if d.heartbeatCount%4 == 0 {
				d.checkForChanges(ctx)
			}
		}
	}
}

func (d *Daemon) detectWithDeviceName() []RuntimeInfo {
	runtimes := DetectRuntimesFast()
	for i := range runtimes {
		runtimes[i].Name = fmt.Sprintf("%s (%s)", capitalize(runtimes[i].Provider), d.cfg.DeviceName)
	}
	return EnrichOpenclawAgents(runtimes)
}

func (d *Daemon) checkForChanges(ctx context.Context) {
	current := d.detectWithDeviceName()
	if !runtimesChanged(d.lastRuntimes, current) {
		return
	}
	log.Printf("[INFO] runtime changes detected, re-registering...")
	d.reRegister(ctx, current)
}

func (d *Daemon) forceReRegister(ctx context.Context) {
	current := d.detectWithDeviceName()
	d.reRegister(ctx, current)
}

func (d *Daemon) reRegister(ctx context.Context, current []RuntimeInfo) {
	req := RegisterRequest{
		DaemonID:   d.daemonID,
		DeviceName: d.cfg.DeviceName,
		CLIVersion: d.cfg.CLIVersion,
		Runtimes:   current,
	}

	resp, err := d.client.Register(ctx, req)
	if err != nil {
		log.Printf("[WARN] re-register failed: %v", err)
		return
	}
	d.lastRuntimes = current
	d.registeredRuntimes = resp.Runtimes
	log.Printf("[INFO] re-registered %d runtime(s)", len(d.registeredRuntimes))
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
		// Check agent IDs changed
		for i, a := range r.Agents {
			if i >= len(prev.Agents) || a.ID != prev.Agents[i].ID || a.Bindings != prev.Agents[i].Bindings {
				return true
			}
		}
	}
	return false
}

func (d *Daemon) sendHeartbeats(ctx context.Context) {
	offlineProviders := make(map[string]bool)
	for _, r := range d.lastRuntimes {
		if r.Status == "offline" {
			offlineProviders[r.Provider] = true
		}
	}

	needReRegister := false
	for _, rt := range d.registeredRuntimes {
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
		d.forceReRegister(ctx)
	}
}

func (d *Daemon) deregister() {
	if len(d.registeredRuntimes) == 0 {
		return
	}

	ids := make([]int64, len(d.registeredRuntimes))
	for i, rt := range d.registeredRuntimes {
		ids[i] = rt.ID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.client.Deregister(ctx, ids); err != nil {
		log.Printf("[WARN] deregister failed: %v", err)
		return
	}

	log.Printf("[INFO] deregistered %d runtime(s)", len(ids))
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
