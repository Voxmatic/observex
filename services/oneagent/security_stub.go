//go:build linux && !ebpf_generated

// services/oneagent/security_stub.go
//
// Build-time stub for the types that bpf2go generates in security_bpfel.go.
// Mirrors the exact pattern of netflow_stub.go — see that file for full docs.
//
// Uses *ciliumebpf.Map so ringbuf.NewReader(objs.securityMaps.SecEvents)
// compiles without error. loadSecurityObjects() returns an error immediately
// so this code path is never reached at runtime without generated files.

package main

import (
	"fmt"

	ciliumebpf "github.com/cilium/ebpf"
)

type securityPrograms struct {
	TraceepointSyscallsSysEnterExecve  *ciliumebpf.Program `ebpf:"tracepoint__syscalls__sys_enter_execve"`
	TraceepointSyscallsSysEnterConnect *ciliumebpf.Program `ebpf:"tracepoint__syscalls__sys_enter_connect"`
	TraceepointSyscallsSysEnterOpenat  *ciliumebpf.Program `ebpf:"tracepoint__syscalls__sys_enter_openat"`
	TraceepointSyscallsSysEnterPtrace  *ciliumebpf.Program `ebpf:"tracepoint__syscalls__sys_enter_ptrace"`
}

type securityMaps struct {
	SecEvents *ciliumebpf.Map `ebpf:"sec_events"`
}

type securityObjects struct {
	securityPrograms
	securityMaps
}

func (o *securityObjects) Close() error {
	if o.TraceepointSyscallsSysEnterExecve != nil {
		o.TraceepointSyscallsSysEnterExecve.Close()
	}
	if o.TraceepointSyscallsSysEnterConnect != nil {
		o.TraceepointSyscallsSysEnterConnect.Close()
	}
	if o.TraceepointSyscallsSysEnterOpenat != nil {
		o.TraceepointSyscallsSysEnterOpenat.Close()
	}
	if o.TraceepointSyscallsSysEnterPtrace != nil {
		o.TraceepointSyscallsSysEnterPtrace.Close()
	}
	if o.SecEvents != nil {
		o.SecEvents.Close()
	}
	return nil
}

func loadSecurityObjects(_ *securityObjects, _ *ciliumebpf.CollectionOptions) error {
	return fmt.Errorf("security eBPF programs not embedded: run 'make -C ebpf generate' first")
}
