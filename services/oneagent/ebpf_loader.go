//go:build linux

// services/oneagent/ebpf_loader.go
//
// ObserveX OneAgent — eBPF ring buffer loader.
//
// This file wires the compiled BPF programs (netflow.bpf.c) to the Go runtime:
//   1. Lifts the memlock RLIMIT so the kernel allows BPF map allocation.
//   2. Loads the BPF ELF bytecode embedded by bpf2go (netflow_bpfel.go).
//   3. Attaches five kernel probes / tracepoints.
//   4. Reads NetFlowEvent records from the ring buffer in a tight loop.
//   5. Translates each event into a models.TopoEdge and sends it to the ingestor
//      via a.sender.SendTopoEdge(), which is the same path used by the /proc fallback.
//
// BUILD REQUIREMENT
// -----------------
// The generated file netflow_bpfel.go (produced by `make generate` in ebpf/)
// MUST exist before this file compiles. It exports loadNetflowObjects().
// If you haven't run `make generate` yet, the build will fail with:
//   "undefined: loadNetflowObjects"
// Run:
//   make -C services/oneagent/../../ebpf generate
//
// FALLBACK
// --------
// If the kernel does not support eBPF (< 4.18), if BTF is missing, or if
// any probe attachment fails, we fall back to the /proc/net/tcp reader
// already implemented in main.go:runNetflowFallback.

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"go.uber.org/zap"

	"github.com/observex/platform/pkg/models"
)

// ─── NetFlowEvent ─────────────────────────────────────────────────────────────
//
// Mirror of the C struct in ebpf/src/netflow.bpf.c.
// Field order, sizes, and padding MUST be identical — binary.Read maps bytes
// directly onto this struct with no schema negotiation.
//
// C struct layout (natural alignment, no __packed__):
//   src_ip      __u32   4 bytes  offset  0
//   dst_ip      __u32   4 bytes  offset  4
//   src_port    __u16   2 bytes  offset  8
//   dst_port    __u16   2 bytes  offset 10
//   pid         __u32   4 bytes  offset 12
//   tid         __u32   4 bytes  offset 16
//   [implicit]  [4]byte 4 bytes  offset 20  ← compiler inserts for __u64 alignment
//   bytes_sent  __u64   8 bytes  offset 24
//   bytes_recv  __u64   8 bytes  offset 32
//   latency_ns  __u64   8 bytes  offset 40
//   event_type  __u8    1 byte   offset 48
//   protocol    __u8    1 byte   offset 49
//   _pad        [6]u8   6 bytes  offset 50  ← explicit in C, fills to 8-byte boundary
//   comm        [16]u8 16 bytes  offset 56
//   Total: 72 bytes
//
// Go's binary.Read requires the struct layout to match byte-for-byte.
// The _AlignPad field below corresponds to the implicit 4-byte gap the C compiler
// inserts between tid (offset 16) and bytes_sent (offset 24).

type NetFlowEvent struct {
	SrcIP     uint32   // offset  0: IPv4 source address, host byte order
	DstIP     uint32   // offset  4: IPv4 destination address, host byte order
	SrcPort   uint16   // offset  8: source port, host byte order
	DstPort   uint16   // offset 10: destination port, host byte order
	PID       uint32   // offset 12: process ID
	TID       uint32   // offset 16: thread ID
	_AlignPad [4]byte  // offset 20: mirrors 4-byte implicit C padding for uint64 alignment
	BytesSent uint64   // offset 24: cumulative bytes sent on this flow
	BytesRecv uint64   // offset 32: cumulative bytes received
	LatencyNs uint64   // offset 40: connect() → ESTABLISHED latency, nanoseconds
	EventType uint8    // offset 48: EVT_CONNECT=1, EVT_ACCEPT=2, EVT_CLOSE=3, EVT_DATA=4
	Protocol  uint8    // offset 49: 6=TCP
	Pad       [6]byte  // offset 50: mirrors explicit __u8 _pad[6] in C struct
	Comm      [16]byte // offset 56: null-terminated process name (TASK_COMM_LEN=16)
	// Total: 72 bytes — matches sizeof(struct NetFlowEvent) in C
}

// evtXxx matches the C #define constants in netflow.bpf.c
const (
	evtConnect = uint8(1)
	evtAccept  = uint8(2)
	evtClose   = uint8(3)
	evtData    = uint8(4)
)

// SrcIPString returns the dotted-quad IPv4 string for the source address.
func (e *NetFlowEvent) SrcIPString() string {
	return uint32ToIP(e.SrcIP)
}

// DstIPString returns the dotted-quad IPv4 string for the destination address.
func (e *NetFlowEvent) DstIPString() string {
	return uint32ToIP(e.DstIP)
}

// CommString returns the null-terminated process name as a Go string.
func (e *NetFlowEvent) CommString() string {
	n := bytes.IndexByte(e.Comm[:], 0)
	if n < 0 {
		n = 16
	}
	return string(e.Comm[:n])
}

func uint32ToIP(ip uint32) string {
	b := [4]byte{byte(ip >> 24), byte(ip >> 16), byte(ip >> 8), byte(ip)}
	return net.IP(b[:]).String()
}

// ─── runEBPFProbe ─────────────────────────────────────────────────────────────
//
// Entry point called from main.go. Replaces the stub.
// Tries to load eBPF; falls back to /proc if anything fails.

func (a *OneAgent) runEBPFProbe(ctx context.Context) {
	a.logger.Info("starting eBPF network probe")

	if err := a.runEBPFLoader(ctx); err != nil {
		a.logger.Warn("eBPF loader failed — falling back to /proc/net/tcp",
			zap.Error(err),
			zap.String("hint", "ensure kernel ≥5.8, BTF enabled, and CAP_BPF/CAP_SYS_ADMIN"),
		)
		a.runNetflowFallback(ctx)
	}
}

// ─── runEBPFLoader ────────────────────────────────────────────────────────────

func (a *OneAgent) runEBPFLoader(ctx context.Context) error {
	// ── 1. Lift memlock limit ─────────────────────────────────────────────────
	// Kernels < 5.11 require RLIMIT_MEMLOCK to be raised for BPF map allocation.
	// cilium/ebpf's rlimit.RemoveMemlock() sets it to RLIM_INFINITY safely.
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove memlock rlimit: %w", err)
	}

	// ── 2. Load BPF objects (programs + maps) ─────────────────────────────────
	// loadNetflowObjects is generated by bpf2go in netflow_bpfel.go.
	// It reads the embedded ELF bytecode and loads it into the kernel via BPF syscalls.
	objs := netflowObjects{}
	if err := loadNetflowObjects(&objs, nil); err != nil {
		return fmt.Errorf("load BPF objects: %w — run 'make -C ebpf generate' first", err)
	}
	defer objs.Close()

	a.logger.Info("BPF objects loaded",
		zap.String("programs", "tcp_connect, tcp_connect_ret, inet_sock_set_state, tcp_sendmsg, tcp_recvmsg"),
	)

	// ── 3. Attach probes ──────────────────────────────────────────────────────
	links, err := attachProbes(objs, a.logger)
	if err != nil {
		return err
	}
	defer func() {
		for _, l := range links {
			l.Close()
		}
	}()

	// ── 4. Open ring buffer reader ────────────────────────────────────────────
	rd, err := ringbuf.NewReader(objs.netflowMaps.Events)
	if err != nil {
		return fmt.Errorf("open ring buffer: %w", err)
	}

	// Close the reader when the context is cancelled — this unblocks rd.Read().
	go func() {
		<-ctx.Done()
		rd.Close()
	}()

	a.logger.Info("eBPF probes attached and ring buffer open — observing TCP flows")

	// ── 5. Event read loop ────────────────────────────────────────────────────
	return a.readRingBuffer(ctx, rd)
}

// ─── attachProbes ─────────────────────────────────────────────────────────────

func attachProbes(objs netflowObjects, log *zap.Logger) ([]link.Link, error) {
	var links []link.Link
	var err error
	var l link.Link

	// kprobe/tcp_connect — must succeed (primary connect tracking)
	l, err = link.Kprobe("tcp_connect", objs.netflowPrograms.HandleTcpConnect, nil)
	if err != nil {
		return nil, fmt.Errorf("attach kprobe/tcp_connect: %w", err)
	}
	links = append(links, l)
	log.Debug("attached kprobe/tcp_connect")

	// kretprobe/tcp_connect — must succeed (emits EVT_CONNECT)
	l, err = link.Kretprobe("tcp_connect", objs.netflowPrograms.HandleTcpConnectRet, nil)
	if err != nil {
		closeLinks(links)
		return nil, fmt.Errorf("attach kretprobe/tcp_connect: %w", err)
	}
	links = append(links, l)
	log.Debug("attached kretprobe/tcp_connect")

	// tracepoint/sock/inet_sock_set_state — must succeed (handles ESTABLISHED + CLOSE)
	l, err = link.Tracepoint("sock", "inet_sock_set_state",
		objs.netflowPrograms.HandleInetSockSetState, nil)
	if err != nil {
		closeLinks(links)
		return nil, fmt.Errorf("attach tracepoint/sock:inet_sock_set_state: %w", err)
	}
	links = append(links, l)
	log.Debug("attached tracepoint/sock/inet_sock_set_state")

	// kprobe/tcp_sendmsg — non-fatal (byte counting only)
	l, err = link.Kprobe("tcp_sendmsg", objs.netflowPrograms.HandleTcpSendmsg, nil)
	if err != nil {
		log.Warn("attach kprobe/tcp_sendmsg failed — bytes_sent will be 0", zap.Error(err))
	} else {
		links = append(links, l)
		log.Debug("attached kprobe/tcp_sendmsg")
	}

	// kprobe/tcp_recvmsg — non-fatal (byte counting only)
	l, err = link.Kprobe("tcp_recvmsg", objs.netflowPrograms.HandleTcpRecvmsg, nil)
	if err != nil {
		log.Warn("attach kprobe/tcp_recvmsg failed — bytes_recv will be 0", zap.Error(err))
	} else {
		links = append(links, l)
		log.Debug("attached kprobe/tcp_recvmsg")
	}

	return links, nil
}

func closeLinks(links []link.Link) {
	for _, l := range links {
		if l != nil {
			l.Close()
		}
	}
}

// ─── readRingBuffer ───────────────────────────────────────────────────────────

func (a *OneAgent) readRingBuffer(ctx context.Context, rd *ringbuf.Reader) error {
	for {
		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil // clean shutdown via ctx.Done()
			}
			a.logger.Warn("ring buffer read error", zap.Error(err))
			// Brief pause before retrying to avoid spinning on persistent errors
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		var evt NetFlowEvent
		if err := binary.Read(
			bytes.NewReader(record.RawSample),
			binary.LittleEndian, // BPF ring buffer is always little-endian on x86
			&evt,
		); err != nil {
			a.logger.Debug("parse ring buffer record failed",
				zap.Int("raw_len", len(record.RawSample)),
				zap.Error(err),
			)
			continue
		}

		a.processNetFlowEvent(ctx, &evt)
	}
}

// ─── processNetFlowEvent ──────────────────────────────────────────────────────

func (a *OneAgent) processNetFlowEvent(ctx context.Context, evt *NetFlowEvent) {
	srcIP := evt.SrcIPString()
	dstIP := evt.DstIPString()

	// Filter — skip loopback, unroutable, and non-pod addresses
	if isSkippableIP(srcIP) || isSkippableIP(dstIP) {
		return
	}

	a.logger.Debug("ebpf event",
		zap.Uint8("type", evt.EventType),
		zap.String("src", fmt.Sprintf("%s:%d", srcIP, evt.SrcPort)),
		zap.String("dst", fmt.Sprintf("%s:%d", dstIP, evt.DstPort)),
		zap.Uint64("bytes_sent", evt.BytesSent),
		zap.Uint64("bytes_recv", evt.BytesRecv),
		zap.Uint64("latency_ns", evt.LatencyNs),
		zap.String("comm", evt.CommString()),
	)

	switch evt.EventType {
	case evtConnect, evtAccept:
		a.onConnect(ctx, evt, srcIP, dstIP)

	case evtClose:
		a.onClose(ctx, evt, srcIP, dstIP)

	case evtData:
		a.onData(ctx, evt, srcIP, dstIP)
	}
}

// ─── Event handlers ───────────────────────────────────────────────────────────

func (a *OneAgent) onConnect(ctx context.Context, evt *NetFlowEvent, srcIP, dstIP string) {
	a.mu.RLock()
	srcSvc := a.resolveIPToService(srcIP)
	dstSvc := a.resolveIPToService(dstIP)
	a.mu.RUnlock()

	if srcSvc == nil || dstSvc == nil || srcSvc.ID == dstSvc.ID {
		return
	}

	latencyMs := float64(evt.LatencyNs) / 1e6

	edge := models.TopoEdge{
		ID:           srcSvc.ID + "->" + dstSvc.ID,
		SourceID:     srcSvc.ID,
		TargetID:     dstSvc.ID,
		Protocol:     "tcp",
		AvgLatencyMs: latencyMs,
		UpdatedAt:    time.Now(),
	}

	// Track the live flow for byte aggregation on close
	flowKey := fmt.Sprintf("%s:%d->%s:%d", srcIP, evt.SrcPort, dstIP, evt.DstPort)
	a.mu.Lock()
	a.netConns[flowKey] = &NetFlow{
		SrcIP:     srcIP,
		DstIP:     dstIP,
		DstPort:   int(evt.DstPort),
		Protocol:  "tcp",
		UpdatedAt: time.Now(),
	}
	a.mu.Unlock()

	a.sender.SendTopoEdge(ctx, edge)
}

func (a *OneAgent) onClose(ctx context.Context, evt *NetFlowEvent, srcIP, dstIP string) {
	a.mu.RLock()
	srcSvc := a.resolveIPToService(srcIP)
	dstSvc := a.resolveIPToService(dstIP)
	a.mu.RUnlock()

	if srcSvc == nil || dstSvc == nil || srcSvc.ID == dstSvc.ID {
		return
	}

	// Duration estimate: use the flow's bytes/sec as a proxy for call rate.
	// BytesSent and BytesRecv are cumulative over the entire flow lifetime.
	bps := float64(evt.BytesSent+evt.BytesRecv) // total bytes — caller should normalise

	edge := models.TopoEdge{
		ID:          srcSvc.ID + "->" + dstSvc.ID,
		SourceID:    srcSvc.ID,
		TargetID:    dstSvc.ID,
		Protocol:    "tcp",
		BytesPerSec: bps,
		UpdatedAt:   time.Now(),
	}
	a.sender.SendTopoEdge(ctx, edge)

	// Clean up the live flow entry
	flowKey := fmt.Sprintf("%s:%d->%s:%d", srcIP, evt.SrcPort, dstIP, evt.DstPort)
	a.mu.Lock()
	delete(a.netConns, flowKey)
	a.mu.Unlock()
}

func (a *OneAgent) onData(ctx context.Context, evt *NetFlowEvent, srcIP, dstIP string) {
	// DATA events are periodic snapshots of an active flow.
	// Update byte counters on the existing edge without emitting a new connect.
	a.mu.RLock()
	srcSvc := a.resolveIPToService(srcIP)
	dstSvc := a.resolveIPToService(dstIP)
	a.mu.RUnlock()

	if srcSvc == nil || dstSvc == nil {
		return
	}

	edge := models.TopoEdge{
		ID:          srcSvc.ID + "->" + dstSvc.ID,
		SourceID:    srcSvc.ID,
		TargetID:    dstSvc.ID,
		Protocol:    "tcp",
		BytesPerSec: float64(evt.BytesSent + evt.BytesRecv),
		UpdatedAt:   time.Now(),
	}
	a.sender.SendTopoEdge(ctx, edge)
}

// ─── IP resolution ────────────────────────────────────────────────────────────
//
// resolveIPToService maps an IPv4 address to a Service discovered by the agent.
// The agent maintains a.services populated by K8s watch and /proc scan.
// MUST be called with a.mu held (RLock or Lock).

func (a *OneAgent) resolveIPToService(ip string) *models.Service {
	for _, svc := range a.services {
		// Check declared endpoints (from K8s service/pod spec)
		for _, ep := range svc.Endpoints {
			if ep.Address == ip {
				return svc
			}
		}
		// Check pod_ip label injected by K8s watcher
		if svc.Labels != nil {
			if podIP, ok := svc.Labels["pod_ip"]; ok && podIP == ip {
				return svc
			}
		}
	}
	return nil
}

// ─── IP filter ────────────────────────────────────────────────────────────────
//
// isSkippableIP returns true for addresses we never want to trace:
// loopback, link-local, unspecified, and multicast.

var (
	skipNets     []*net.IPNet
	skipNetsOnce sync.Once
)

func initSkipNets() {
	cidrs := []string{
		"127.0.0.0/8",    // loopback
		"169.254.0.0/16", // link-local
		"0.0.0.0/8",      // unspecified
		"224.0.0.0/4",    // multicast
		"255.255.255.255/32", // broadcast
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
	}
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			skipNets = append(skipNets, n)
		}
	}
}

func isSkippableIP(ipStr string) bool {
	skipNetsOnce.Do(initSkipNets)

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true
	}
	for _, n := range skipNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ─── Kernel support check ────────────────────────────────────────────────────
//
// isEBPFSupported checks for:
//  1. /sys/kernel/debug/tracing — required for kprobes
//  2. /sys/kernel/btf/vmlinux  — required for CO-RE type resolution
//
// Both must be present for the full program to work.
// Without BTF, programs compiled against a bundled vmlinux.h may load
// but CO-RE field relocations will fail at load time.

func isEBPFSupported() bool {
	if _, err := os.Stat("/sys/kernel/debug/tracing"); err != nil {
		return false
	}
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err != nil {
		return false
	}
	return true
}
