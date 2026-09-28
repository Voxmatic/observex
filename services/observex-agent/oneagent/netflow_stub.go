//go:build linux && !ebpf_generated

// services/oneagent/netflow_stub.go
//
// Build-time stub for the types that bpf2go generates in netflow_bpfel.go.
//
// This file is compiled when the build tag "ebpf_generated" is NOT set —
// i.e., before `go generate` or `make generate` has been run.
//
// Its sole purpose is to make the Go toolchain happy so `go build ./...`
// works on a fresh checkout without needing clang/llvm installed.
//
// When netflow_bpfel.go EXISTS (generated), the real types override these stubs
// because bpf2go generates files WITHOUT the `!ebpf_generated` build constraint.
// The Go build system picks the real file when it is present.
//
// In production Docker builds, CI, and developer machines with clang:
//   make -C ebpf generate     # generates netflow_bpfel.go
//   go build ./...            # uses the real generated types — stub ignored
//
// On machines without clang (pure review, IDE navigation):
//   go build ./...            # uses this stub — eBPF falls back to /proc
//
// NOTE: The stub's loadNetflowObjects always returns an error, which causes
// runEBPFLoader to fail immediately and fall back to runNetflowFallback.
// The binary is functional; it just doesn't use kernel eBPF programs.

package main

import "fmt"

// cilium/ebpf types referenced by ebpf_loader.go
// These are minimal stubs — the real types come from github.com/cilium/ebpf.

import ciliumebpf "github.com/cilium/ebpf"

// netflowPrograms holds all BPF program handles for the netflow collection.
// Field names match the SEC("...") names in netflow.bpf.c with camelCase.
type netflowPrograms struct {
	HandleTcpConnect       *ciliumebpf.Program `ebpf:"handle_tcp_connect"`
	HandleTcpConnectRet    *ciliumebpf.Program `ebpf:"handle_tcp_connect_ret"`
	HandleInetSockSetState *ciliumebpf.Program `ebpf:"handle_inet_sock_set_state"`
	HandleTcpSendmsg       *ciliumebpf.Program `ebpf:"handle_tcp_sendmsg"`
	HandleTcpRecvmsg       *ciliumebpf.Program `ebpf:"handle_tcp_recvmsg"`
}

// netflowMaps holds all BPF map handles for the netflow collection.
type netflowMaps struct {
	Events           *ciliumebpf.Map `ebpf:"events"`
	ConnectArgsMap   *ciliumebpf.Map `ebpf:"connect_args_map"`
	ActiveFlows      *ciliumebpf.Map `ebpf:"active_flows"`
}

// netflowObjects bundles programs and maps — loaded together as a unit.
type netflowObjects struct {
	netflowPrograms
	netflowMaps
}

// Close releases all kernel resources held by these objects.
func (o *netflowObjects) Close() {
	if o.netflowPrograms.HandleTcpConnect != nil {
		o.HandleTcpConnect.Close()
	}
	if o.netflowPrograms.HandleTcpConnectRet != nil {
		o.HandleTcpConnectRet.Close()
	}
	if o.netflowPrograms.HandleInetSockSetState != nil {
		o.HandleInetSockSetState.Close()
	}
	if o.netflowPrograms.HandleTcpSendmsg != nil {
		o.HandleTcpSendmsg.Close()
	}
	if o.netflowPrograms.HandleTcpRecvmsg != nil {
		o.HandleTcpRecvmsg.Close()
	}
	if o.netflowMaps.Events != nil {
		o.Events.Close()
	}
	if o.netflowMaps.ConnectArgsMap != nil {
		o.ConnectArgsMap.Close()
	}
	if o.netflowMaps.ActiveFlows != nil {
		o.ActiveFlows.Close()
	}
}

// loadNetflowObjects is the stub for the bpf2go-generated loader.
// Returns an error so runEBPFLoader falls back to /proc gracefully.
func loadNetflowObjects(objs *netflowObjects, opts *ciliumebpf.CollectionOptions) error {
	return fmt.Errorf(
		"BPF bytecode not embedded: run 'make -C ebpf generate' to compile " +
			"netflow.bpf.c and generate netflow_bpfel.go, then rebuild",
	)
}
