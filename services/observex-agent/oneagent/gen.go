//go:build ignore

// services/oneagent/gen.go
//
// go:generate directive for bpf2go.
// Run from the repo root:
//
//   cd services/oneagent && go generate
//   # or
//   make -C ../../ebpf generate
//
// bpf2go compiles ebpf/src/netflow.bpf.c and generates:
//   netflow_bpfel.go   little-endian (x86_64, arm64)
//   netflow_bpfeb.go   big-endian    (s390x, mips)
//
// Each file exports the following types and functions used by ebpf_loader.go:
//
//   type netflowPrograms struct {
//       HandleTcpConnect        *ebpf.Program
//       HandleTcpConnectRet     *ebpf.Program
//       HandleInetSockSetState  *ebpf.Program
//       HandleTcpSendmsg        *ebpf.Program
//       HandleTcpRecvmsg        *ebpf.Program
//   }
//   type netflowMaps struct {
//       Events       *ebpf.Map
//       ConnectStart *ebpf.Map
//       ActiveFlows  *ebpf.Map
//   }
//   type netflowObjects struct { netflowPrograms; netflowMaps }
//   func loadNetflowObjects(*netflowObjects, *ebpf.CollectionOptions) error

package main

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go \
//   -go-package main \
//   -output-dir . \
//   -cc clang \
//   -strip llvm-strip \
//   -cflags "-O2 -g -Wall -D__TARGET_ARCH_x86 -D__BPF_TRACING__ -I../../ebpf/headers -I/usr/include/bpf" \
//   netflow \
//   ../../ebpf/src/netflow.bpf.c
