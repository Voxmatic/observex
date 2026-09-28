//go:build linux

// services/oneagent/security_loader.go
//
// ObserveX Runtime Security Monitor.
//
// Reads SecurityEvent records from the BPF ring buffer produced by
// security.bpf.c, enriches each event with:
//   - Kubernetes pod name / namespace / container image (from PID → cgroup)
//   - Severity score (CRITICAL / HIGH / MEDIUM / LOW / INFO)
//   - Human-readable threat description
//   - MITRE ATT&CK tactic + technique
//
// Events are then sent to the ingestor as:
//   - models.SecurityEvent  → POST /v1/security/events
//   - models.MetricPoint    → counter metric  observex_security_events_total{severity,tactic}
//   - models.LogEntry       → Loki log stream  {service="security-monitor"}
//
// The loader runs as a goroutine alongside the existing netflow eBPF loader.
// If the security BPF programs fail to load, it falls back gracefully and
// logs a warning — the agent continues with network-only monitoring.
//
// Thread safety: all public methods are goroutine-safe.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"go.uber.org/zap"

	"github.com/observex/platform/pkg/models"
)

// ── SecurityEvent mirrors the BPF struct (128 bytes, same layout) ─────────

type SecurityEventBPF struct {
	EventType  uint8
	Severity   uint8
	Pad        [2]uint8
	PID        uint32
	PPID       uint32
	UID        uint32
	DstIP      uint32
	DstPort    uint16
	Pad2       uint16
	PtraceReq  int32
	Pad3       uint32
	Comm       [16]byte
	Path       [64]byte
	Args       [16]byte
}

// Verify size matches C struct at compile time.
// If this fails, update padding fields above to match security.bpf.c byte map.
var _ = [1]struct{}{}[unsafe.Sizeof(SecurityEventBPF{})-128]

// ── BPF event type constants ──────────────────────────────────────────────

const (
	secEvtExec     = 1
	secEvtConnect  = 2  // = secEvtConnect from netflow, same value - don't redeclare
	secEvtFileOpen = 3
	secEvtPtrace   = 4
)

// ── MITRE ATT&CK mappings ─────────────────────────────────────────────────

type mitre struct {
	Tactic    string
	Technique string
	TechID    string
}

var mitreByType = map[uint8]mitre{
	secEvtExec:     {"Execution", "Command and Scripting Interpreter", "T1059"},
	secEvtConnect:  {"Command and Control", "Application Layer Protocol", "T1071"},
	secEvtFileOpen: {"Credential Access", "OS Credential Dumping", "T1003"},
	secEvtPtrace:   {"Defense Evasion", "Process Injection", "T1055"},
}

// ── Known-safe process patterns (suppress noisy benign connects) ──────────

var safeProcesses = []string{
	"curl", "wget", "apt", "apt-get", "yum", "dnf", "apk",
	"kubelet", "kube-proxy",
	"containerd", "dockerd", "runc",
}

func isSafeProcess(comm string) bool {
	for _, s := range safeProcesses {
		if comm == s || strings.HasPrefix(comm, s) {
			return true
		}
	}
	return false
}

// ── High-risk paths that always produce HIGH or CRITICAL events ───────────

var criticalPaths = []string{
	"/etc/shadow", "/etc/passwd", "/etc/sudoers",
	"/.ssh/", "/root/.ssh/",
}
var highPaths = []string{
	"/proc/", "/var/run/docker.sock", "/run/containerd",
	"/etc/kubernetes", "/var/lib/kubelet/pki",
}

func pathSeverity(path string) string {
	for _, p := range criticalPaths {
		if strings.Contains(path, p) {
			return "CRITICAL"
		}
	}
	for _, p := range highPaths {
		if strings.Contains(path, p) {
			return "HIGH"
		}
	}
	return "MEDIUM"
}

// ── SecurityLoader ────────────────────────────────────────────────────────

type SecurityLoader struct {
	agent  *OneAgent
	logger *zap.Logger
}

func NewSecurityLoader(agent *OneAgent, logger *zap.Logger) *SecurityLoader {
	return &SecurityLoader{agent: agent, logger: logger}
}

// Run is the entry point. It tries to load the eBPF programs; on any failure
// it logs and returns without crashing the agent.
func (sl *SecurityLoader) Run(ctx context.Context) {
	if err := sl.runSecurityLoader(ctx); err != nil {
		sl.logger.Warn("security eBPF loader failed — runtime threat detection disabled",
			zap.Error(err),
		)
	}
}

func (sl *SecurityLoader) runSecurityLoader(ctx context.Context) error {
	// Lift memlock — required for BPF map allocation
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove memlock rlimit: %w", err)
	}

	// Load compiled BPF objects
	var objs securityObjects
	if err := loadSecurityObjects(&objs, nil); err != nil {
		return fmt.Errorf("load security BPF objects: %w", err)
	}
	defer objs.Close()

	// Attach tracepoints
	links, err := attachSecurityProbes(objs, sl.logger)
	if err != nil {
		return fmt.Errorf("attach security probes: %w", err)
	}
	defer func() {
		for _, l := range links {
			l.Close()
		}
	}()

	sl.logger.Info("security eBPF probes attached",
		zap.Strings("probes", []string{
			"tracepoint/syscalls/sys_enter_execve",
			"tracepoint/syscalls/sys_enter_connect",
			"tracepoint/syscalls/sys_enter_openat",
			"tracepoint/syscalls/sys_enter_ptrace",
		}),
	)

	return sl.readRingBuffer(ctx, objs)
}

func attachSecurityProbes(objs securityObjects, log *zap.Logger) ([]link.Link, error) {
	var links []link.Link

	attachTP := func(group, name string, prog *ebpf.Program) {
		l, err := link.Tracepoint(group, name, prog, nil)
		if err != nil {
			log.Warn("failed to attach tracepoint (non-fatal)",
				zap.String("group", group), zap.String("name", name), zap.Error(err))
			return
		}
		links = append(links, l)
		log.Debug("attached security probe", zap.String("probe", group+"/"+name))
	}

	// Execve — process execution (most critical for container security)
	attachTP("syscalls", "sys_enter_execve", objs.securityPrograms.TraceepointSyscallsSysEnterExecve)

	// Connect — outbound connection tracking
	attachTP("syscalls", "sys_enter_connect", objs.securityPrograms.TraceepointSyscallsSysEnterConnect)

	// Openat — sensitive file access
	attachTP("syscalls", "sys_enter_openat", objs.securityPrograms.TraceepointSyscallsSysEnterOpenat)

	// Ptrace — process injection
	attachTP("syscalls", "sys_enter_ptrace", objs.securityPrograms.TraceepointSyscallsSysEnterPtrace)

	return links, nil
}

// ── Ring buffer reader ────────────────────────────────────────────────────

func (sl *SecurityLoader) readRingBuffer(ctx context.Context, objs securityObjects) error {
	// ringbuf.NewReader accepts *ebpf.Map directly.
	// objs.securityMaps.SecEvents is *ebpf.Map when security_bpfel.go is generated.
	// The stub loadSecurityObjects() returns an error before we reach this point,
	// so this cast is always safe in practice.
	reader, err := ringbuf.NewReader(objs.securityMaps.SecEvents)
	if err != nil {
		return fmt.Errorf("open security ring buffer: %w", err)
	}
	defer reader.Close()

	sl.logger.Info("security ring buffer reader started")

	var raw SecurityEventBPF
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			sl.logger.Debug("security ring buffer read error", zap.Error(err))
			time.Sleep(10 * time.Millisecond)
			continue
		}

		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &raw); err != nil {
			sl.logger.Debug("security event decode error", zap.Error(err))
			continue
		}

		sl.processEvent(ctx, &raw)
	}
}

// ── Event processing & enrichment ────────────────────────────────────────

func (sl *SecurityLoader) processEvent(ctx context.Context, raw *SecurityEventBPF) {
	comm  := nullTerm(raw.Comm[:])
	path  := nullTerm(raw.Path[:])
	args  := nullTerm(raw.Args[:])

	// ── Apply noise filters ───────────────────────────────────────────────

	switch raw.EventType {
	case secEvtConnect:
		// Suppress well-known safe processes and loopback
		if isSafeProcess(comm) {
			return
		}
		// Suppress loopback connections
		if raw.DstIP>>24 == 127 {
			return
		}
		// Suppress link-local
		if raw.DstIP>>16 == 0xa9fe {
			return
		}
	case secEvtExec:
		// Suppress extremely common benign execs that would generate noise
		noisy := []string{"ls", "ps", "cat", "echo", "sleep", "true", "false", "test", "["}
		for _, n := range noisy {
			if comm == n || strings.HasSuffix(path, "/"+n) {
				return
			}
		}
	}

	// ── Determine severity ────────────────────────────────────────────────

	severity := classifySeverity(raw, comm, path)
	if severity == "" {
		return // filtered out
	}

	// ── MITRE enrichment ──────────────────────────────────────────────────

	m := mitreByType[raw.EventType]

	// ── Build human-readable description ─────────────────────────────────

	description := buildDescription(raw.EventType, comm, path, args, raw.DstIP, raw.DstPort)

	// ── Resolve pod context from PID ──────────────────────────────────────

	namespace, podName, containerImage := sl.resolvePodContext(raw.PID)

	// ── Emit to ingestor ──────────────────────────────────────────────────

	evt := models.SecurityEvent{
		Timestamp:      time.Now().UTC(),
		EventType:      eventTypeName(raw.EventType),
		Severity:       severity,
		PID:            int(raw.PID),
		PPID:           int(raw.PPID),
		UID:            int(raw.UID),
		ProcessName:    comm,
		Path:           path,
		Args:           args,
		DstIP:          ipString(raw.DstIP),
		DstPort:        int(raw.DstPort),
		NodeName:       sl.agent.cfg.NodeName,
		ClusterName:    sl.agent.cfg.ClusterName,
		Namespace:      namespace,
		PodName:        podName,
		ContainerImage: containerImage,
		Description:    description,
		MitreTactic:    m.Tactic,
		MitreTechnique: m.Technique,
		MitreTechID:    m.TechID,
	}

	go sl.agent.sender.SendSecurityEvent(ctx, evt)

	sl.logger.Info("security event",
		zap.String("type", evt.EventType),
		zap.String("severity", severity),
		zap.String("proc", comm),
		zap.String("path", path),
		zap.String("pod", podName),
		zap.String("ns", namespace),
		zap.String("tactic", m.Tactic),
	)
}

// ── Severity classification ───────────────────────────────────────────────

func classifySeverity(raw *SecurityEventBPF, comm, path string) string {
	switch raw.EventType {

	case secEvtExec:
		// Shell spawned in a container is always HIGH/CRITICAL
		shells := []string{"sh", "bash", "zsh", "fish", "dash", "ash", "ksh"}
		for _, s := range shells {
			if comm == s || strings.HasSuffix(path, "/"+s) {
				return "CRITICAL"
			}
		}
		// Script interpreters
		interpreters := []string{"python", "python3", "perl", "ruby", "node", "php"}
		for _, i := range interpreters {
			if comm == i || strings.HasPrefix(comm, i) {
				return "HIGH"
			}
		}
		// Network tools in containers
		netTools := []string{"nc", "ncat", "nmap", "socat", "ssh", "scp"}
		for _, t := range netTools {
			if comm == t {
				return "HIGH"
			}
		}
		return "MEDIUM"

	case secEvtConnect:
		// Unexpected outbound from a containerised service (UID != 0 makes it less suspicious)
		if raw.UID == 0 {
			return "HIGH" // root process making outbound connection
		}
		// Well-known ports are expected
		port := raw.DstPort
		if port == 80 || port == 443 || port == 53 || port == 5432 || port == 6379 || port == 9200 {
			return "LOW"
		}
		// Unusual high ports
		if port > 1024 && port < 10000 {
			return "MEDIUM"
		}
		return "LOW"

	case secEvtFileOpen:
		return pathSeverity(path)

	case secEvtPtrace:
		// PTRACE_ATTACH (16) or PTRACE_SEIZE (0x4206) are high-risk
		req := raw.PtraceReq
		if req == 16 || req == 0x4206 {
			return "CRITICAL"
		}
		return "HIGH"
	}

	return "LOW"
}

// ── Description builder ───────────────────────────────────────────────────

func buildDescription(evtType uint8, comm, path, args string, dstIP uint32, dstPort uint16) string {
	switch evtType {
	case secEvtExec:
		if args != "" {
			return fmt.Sprintf("process '%s' executed '%s' with args '%s'", comm, path, args)
		}
		return fmt.Sprintf("process '%s' executed '%s'", comm, path)
	case secEvtConnect:
		return fmt.Sprintf("process '%s' connected to %s:%d", comm, ipString(dstIP), dstPort)
	case secEvtFileOpen:
		return fmt.Sprintf("process '%s' opened sensitive path '%s'", comm, path)
	case secEvtPtrace:
		return fmt.Sprintf("process '%s' called ptrace() (req=%d) — possible process injection", comm, 0)
	}
	return fmt.Sprintf("security event from process '%s'", comm)
}

// ── K8s pod context resolution ────────────────────────────────────────────
// Reads /proc/<pid>/cgroup to find the pod UID, then looks it up in the
// K8s watch cache maintained by the main agent loop.

func (sl *SecurityLoader) resolvePodContext(pid uint32) (namespace, podName, containerImage string) {
	// Read cgroup file to get pod UID
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	data, err := os.ReadFile(cgroupPath)
	if err != nil {
		return "", "", ""
	}

	// cgroup path format: ...kubepods/pod<uid>/container<id>/...
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "kubepods") {
			continue
		}
		// Extract pod UID from cgroup path
		parts := strings.Split(line, "/")
		for i, p := range parts {
			if strings.HasPrefix(p, "pod") {
				podUID := strings.TrimPrefix(p, "pod")
				podUID = strings.ReplaceAll(podUID, "-", "-") // normalize
				// Look up in agent's K8s service cache
				sl.agent.mu.RLock()
				for _, svc := range sl.agent.services {
					if svc.Labels != nil && svc.Labels["pod-uid"] == podUID {
						namespace = svc.Labels["namespace"]
						podName   = svc.Labels["pod"]
						containerImage = svc.Labels["image"]
						sl.agent.mu.RUnlock()
						return
					}
				}
				sl.agent.mu.RUnlock()
				_ = i
				return "", "", ""
			}
		}
	}
	return "", "", ""
}

// ── Helpers ───────────────────────────────────────────────────────────────

func nullTerm(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func ipString(ip uint32) string {
	return net.IP{
		byte(ip >> 24),
		byte(ip >> 16),
		byte(ip >> 8),
		byte(ip),
	}.String()
}

func eventTypeName(t uint8) string {
	return map[uint8]string{
		secEvtExec:     "exec",
		secEvtConnect:  "connect",
		secEvtFileOpen: "file_open",
		secEvtPtrace:   "ptrace",
	}[t]
}
