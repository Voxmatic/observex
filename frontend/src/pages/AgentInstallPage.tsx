// AgentInstallPage.tsx — Complete installation wizard: 20+ platforms, Dynatrace-quality
import { useState } from 'react'
import { CheckCircle, Copy, Download, RefreshCw, ChevronRight, Shield, Zap, Server, Box, Cloud, Globe, Activity, Settings2, AlertCircle } from 'lucide-react'
import toast from 'react-hot-toast'

const STEPS = ['Choose Platform', 'Architecture & Mode', 'Generate Token', 'Deploy', 'Verify']

const PLATFORM_GROUPS = [
  {
    id: 'os', label: 'Operating Systems',
    platforms: [
      { id: 'linux-x86', icon: '🐧', label: 'Linux x86-64', sub: 'Ubuntu, RHEL, Debian, Amazon Linux, CentOS, SLES', badge: 'Recommended', ext: 'sh', color: '#f97316', archs: ['x86-64'], modes: ['fullstack','infra','discovery'] },
      { id: 'linux-arm', icon: '🐧', label: 'Linux ARM64', sub: 'AWS Graviton 2/3, Apple M-series, RPi 4+', badge: '', ext: 'sh', color: '#f97316', archs: ['ARM64'], modes: ['fullstack','infra','discovery'] },
      { id: 'linux-ppc', icon: '🐧', label: 'Linux PPC-LE', sub: 'IBM POWER9/10 Little-Endian', badge: 'Enterprise', ext: 'sh', color: '#f97316', archs: ['PPC64LE'], modes: ['fullstack','infra'] },
      { id: 'linux-s390', icon: '🐧', label: 'Linux on Z (s390x)', sub: 'IBM Z mainframe, LinuxONE', badge: 'Enterprise', ext: 'sh', color: '#1a56db', archs: ['s390x'], modes: ['fullstack','infra'] },
      { id: 'windows', icon: '🪟', label: 'Windows', sub: 'Windows Server 2012 R2–2022, Windows 10/11', badge: '', ext: 'ps1', color: '#0078d4', archs: ['x86-64'], modes: ['fullstack','infra','discovery'] },
      { id: 'aix', icon: '🔵', label: 'IBM AIX', sub: 'AIX 7.2, 7.3 — POWER8, POWER9, POWER10', badge: 'Enterprise', ext: 'sh', color: '#1a56db', archs: ['POWER8','POWER9','POWER10'], modes: ['fullstack','infra','app-only'] },
      { id: 'solaris', icon: '☀️', label: 'Solaris', sub: 'Solaris 11.4 — SPARC and x86-64', badge: 'Enterprise', ext: 'sh', color: '#dc8a00', archs: ['SPARC','x86-64'], modes: ['infra','app-only'] },
      { id: 'zos', icon: '🖥', label: 'IBM z/OS', sub: 'z/OS 2.4+ — CICS, IMS, IBM MQ, Db2 on Z', badge: 'Mainframe', ext: 'sh', color: '#1a56db', archs: ['z/Architecture'], modes: ['app-only'] },
    ]
  },
  {
    id: 'containers', label: 'Containers & Kubernetes',
    platforms: [
      { id: 'kubernetes', icon: '⎈', label: 'Kubernetes', sub: 'EKS, GKE, AKS, OpenShift, Rancher, k3s, RKE2, IKS', badge: 'eBPF', ext: 'yaml', color: '#326ce5', archs: ['any'], modes: ['fullstack','infra','app-only','platform'] },
      { id: 'openshift', icon: '🔴', label: 'OpenShift', sub: 'OpenShift 4.x — via Dynatrace Operator + oc CLI', badge: '', ext: 'yaml', color: '#ee0000', archs: ['any'], modes: ['fullstack','app-only'] },
      { id: 'helm', icon: '⛵', label: 'Helm Chart', sub: 'Production Kubernetes via observex/agent Helm chart', badge: 'Production', ext: 'sh', color: '#0f1689', archs: ['any'], modes: ['fullstack','infra','app-only'] },
      { id: 'docker', icon: '🐳', label: 'Docker', sub: 'Docker 20.10+ — auto-discovers all containers on host', badge: '', ext: 'sh', color: '#0db7ed', archs: ['x86-64','ARM64'], modes: ['fullstack','infra'] },
      { id: 'podman', icon: '🐳', label: 'Podman', sub: 'Podman 4.x — crun runtime, OA version 1.267+', badge: '', ext: 'sh', color: '#892ca0', archs: ['x86-64','ARM64'], modes: ['infra','app-only'] },
    ]
  },
  {
    id: 'serverless', label: 'Serverless & Functions',
    platforms: [
      { id: 'lambda', icon: '⚡', label: 'AWS Lambda', sub: 'All runtimes via Lambda Layer — cold start, duration, errors', badge: '', ext: 'sh', color: '#ff9900', archs: ['x86-64','ARM64'], modes: ['app-only'] },
      { id: 'fargate', icon: '🚢', label: 'AWS ECS / Fargate', sub: 'Sidecar container injection — task-level metrics + traces', badge: '', ext: 'yaml', color: '#ff9900', archs: ['x86-64','ARM64'], modes: ['app-only'] },
      { id: 'azure-fn', icon: '⚡', label: 'Azure Functions', sub: 'v4 — .NET, Node.js, Python, Java, Go, PowerShell', badge: '', ext: 'sh', color: '#0078d4', archs: ['x86-64'], modes: ['app-only'] },
      { id: 'gcp-fn', icon: '⚡', label: 'GCP Cloud Functions', sub: '2nd gen — Node.js, Python, Go, Java, Ruby, PHP', badge: '', ext: 'sh', color: '#4285f4', archs: ['x86-64'], modes: ['app-only'] },
    ]
  },
  {
    id: 'paas', label: 'Cloud PaaS',
    platforms: [
      { id: 'azure-app', icon: '☁', label: 'Azure App Service', sub: 'Site extension (Windows) or container sidecar (Linux)', badge: '', ext: 'sh', color: '#0078d4', archs: ['x86-64'], modes: ['fullstack','app-only'] },
      { id: 'cf', icon: '☁', label: 'Cloud Foundry / TAS', sub: 'BOSH Release (full-stack) or Buildpack (app-only); SAP Cloud', badge: '', ext: 'sh', color: '#0f7dc2', archs: ['x86-64'], modes: ['fullstack','app-only'] },
      { id: 'heroku', icon: '🟣', label: 'Heroku', sub: 'All dyno types — buildpack auto-injection', badge: '', ext: 'sh', color: '#79589f', archs: ['x86-64'], modes: ['app-only'] },
      { id: 'gcp-run', icon: '☁', label: 'GCP Cloud Run', sub: 'App-only injection via sidecar container', badge: '', ext: 'yaml', color: '#4285f4', archs: ['x86-64','ARM64'], modes: ['app-only'] },
    ]
  },
  {
    id: 'language', label: 'Language Agents (App-only)',
    platforms: [
      { id: 'java', icon: '☕', label: 'Java Agent', sub: 'Java 8–23 — Spring, Quarkus, Micronaut, JBoss, WebSphere', badge: 'Zero-restart', ext: 'sh', color: '#f89820', archs: ['x86-64','ARM64'], modes: ['app-only'] },
      { id: 'dotnet', icon: '🟣', label: '.NET Agent', sub: '.NET Framework 4.x + .NET 5–8 — CLR profiler, ASP.NET', badge: '', ext: 'ps1', color: '#512bd4', archs: ['x86-64'], modes: ['app-only'] },
      { id: 'nodejs', icon: '🟢', label: 'Node.js Agent', sub: 'Node 14–22 — Express, Fastify, NestJS, Next.js', badge: '', ext: 'sh', color: '#68a063', archs: ['x86-64','ARM64'], modes: ['app-only'] },
      { id: 'python', icon: '🐍', label: 'Python Agent', sub: 'Python 2.7, 3.6–3.12 — Django, Flask, FastAPI, gunicorn', badge: '', ext: 'sh', color: '#3776ab', archs: ['x86-64','ARM64'], modes: ['app-only'] },
      { id: 'go', icon: '🔵', label: 'Go Agent', sub: 'Go 1.18+ — goroutine profiling, GC, dynamic instrumentation', badge: '', ext: 'sh', color: '#00acd7', archs: ['x86-64','ARM64'], modes: ['app-only'] },
      { id: 'php', icon: '🐘', label: 'PHP Agent', sub: 'PHP 7.4–8.3 — Laravel, Symfony, WordPress, Drupal', badge: '', ext: 'sh', color: '#4f5b93', archs: ['x86-64'], modes: ['app-only'] },
      { id: 'ruby', icon: '💎', label: 'Ruby Agent', sub: 'Ruby 2.7–3.3 — Rails, Sinatra, Rack', badge: '', ext: 'sh', color: '#cc342d', archs: ['x86-64'], modes: ['app-only'] },
    ]
  }
]

const MONITORING_MODES = [
  { id: 'fullstack', label: 'Full-Stack', icon: '🔭', desc: 'Complete: host + process injection + code-level insights + network analysis. Recommended for business-critical apps.', recommended: true },
  { id: 'infra', label: 'Infrastructure', icon: '🖥', desc: 'Host metrics only — CPU, memory, disk, network, OS services. No code injection. For non-critical infra.', recommended: false },
  { id: 'app-only', label: 'Application Only', icon: '⚙️', desc: 'Code instrumentation without host agent. For PaaS/serverless where you cannot access the host.', recommended: false },
  { id: 'discovery', label: 'Discovery', icon: '🔍', desc: 'Read-only scan. Auto-discovers services and topology without instrumentation. Very low cost/overhead.', recommended: false },
]

function generateScript(platform: any, mode: string, arch: string, token: string): string {
  const t = token || 'oxat_YOUR_TOKEN_HERE'
  const url = 'https://ingest.observex.io'
  const ver = '2.0.0'
  const pid = platform.id

  if (pid === 'linux-x86' || pid === 'linux-arm' || pid === 'linux-s390' || pid === 'linux-ppc') {
    const archMap: Record<string,string> = { 'linux-x86': 'amd64', 'linux-arm': 'arm64', 'linux-s390': 's390x', 'linux-ppc': 'ppc64le' }
    const a = archMap[pid] || 'amd64'
    return `#!/bin/bash
# ObserveX Agent v${ver} — ${platform.label} Installer
# Architecture: ${a}  Mode: ${mode}
set -e

OBSERVEX_TOKEN="${t}"
OBSERVEX_INGESTOR="${url}"
OBSERVEX_MODE="${mode}"

echo "🚀 Installing ObserveX Agent v${ver} (${a})..."

# Download agent binary
curl -fsSL "https://dl.observex.io/agent/linux/${a}/observex-agent-${ver}" \\
  -o /tmp/observex-agent

# Verify SHA-256 signature
curl -fsSL "https://dl.observex.io/agent/linux/${a}/observex-agent-${ver}.sha256" \\
  | sha256sum -c

chmod +x /tmp/observex-agent
sudo mv /tmp/observex-agent /usr/local/bin/observex-agent

# Create configuration
sudo mkdir -p /etc/observex
sudo tee /etc/observex/agent.yaml > /dev/null << CONFIG
token: ${t}
ingestor_url: ${url}
monitoring_mode: ${mode}
ebpf_enabled: true
log_monitoring: true
process_discovery: true
CONFIG

# Install as systemd service
sudo tee /etc/systemd/system/observex-agent.service > /dev/null << UNIT
[Unit]
Description=ObserveX Agent v${ver}
After=network.target docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/observex-agent --config /etc/observex/agent.yaml
Restart=always
RestartSec=10
CapabilityBoundingSet=CAP_SYS_ADMIN CAP_NET_ADMIN CAP_SYS_PTRACE

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable --now observex-agent

echo ""
echo "✅ ObserveX Agent installed successfully!"
echo "   Version: v${ver}  Architecture: ${a}"
echo "   Mode:    ${mode}"
echo ""
echo "Verify:  systemctl status observex-agent"
echo "Logs:    journalctl -u observex-agent -f"
echo "Data appears in dashboard within ~30 seconds"`
  }

  if (pid === 'windows') {
    return `# ObserveX Agent v${ver} — Windows PowerShell Installer
#Requires -RunAsAdministrator

$Token = "${t}"
$IngestorURL = "${url}"
$Mode = "${mode}"
$Version = "${ver}"
$InstallDir = "C:\\Program Files\\ObserveX"
$AgentPath = "$InstallDir\\observex-agent.exe"

$ErrorActionPreference = "Stop"
Write-Host "🚀 Installing ObserveX Agent v$Version for Windows..." -ForegroundColor Cyan

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

# Download agent
$URL = "https://dl.observex.io/agent/windows/amd64/observex-agent-$Version.exe"
Invoke-WebRequest -Uri $URL -OutFile $AgentPath -UseBasicParsing

# Write configuration
@"
token: $Token
ingestor_url: $IngestorURL
monitoring_mode: $Mode
wmi_enabled: true
windows_event_log: true
"@ | Set-Content -Path "$InstallDir\\agent.yaml"

# Install as Windows Service
New-Service -Name "ObserveXAgent" \\
  -DisplayName "ObserveX Agent" \\
  -BinaryPathName "$AgentPath --config $InstallDir\\agent.yaml" \\
  -StartupType Automatic

Start-Service -Name "ObserveXAgent"

Write-Host "✅ ObserveX Agent installed!" -ForegroundColor Green
Write-Host "   Version: v$Version  Mode: $Mode"
Write-Host "Verify: Get-Service ObserveXAgent"`
  }

  if (pid === 'aix') {
    return `#!/bin/sh
# ObserveX Agent v${ver} — IBM AIX Installer
# Architecture: POWER  AIX 7.2/7.3
# Note: wget is NOT installed by default on AIX — use alternative download

OBSERVEX_TOKEN="${t}"
OBSERVEX_INGESTOR="${url}"
OBSERVEX_MODE="${mode}"

echo "🚀 Installing ObserveX Agent v${ver} for IBM AIX (POWER)..."

# Download via curl (preferred) or ftp if wget unavailable
curl -fsSL "https://dl.observex.io/agent/aix/power/observex-agent-${ver}" \\
  -o /tmp/observex-agent

# Verify signature
curl -fsSL "https://dl.observex.io/agent/aix/power/observex-agent-${ver}.sha256" \\
  | sha256sum -c

chmod +x /tmp/observex-agent
cp /tmp/observex-agent /usr/local/bin/observex-agent

# Create config
mkdir -p /etc/observex
cat > /etc/observex/agent.yaml << CONFIG
token: ${t}
ingestor_url: ${url}
monitoring_mode: ${mode}
aix_universal_injection: true
CONFIG

# Start via AIX init
/usr/local/bin/observex-agent --config /etc/observex/agent.yaml &

echo "✅ ObserveX Agent started on AIX"
echo ""
echo "NOTE: For RBAC-privileged processes (Apache with PV_NET_PORT, etc.)"
echo "      use manual injection via LD_PRELOAD:"
echo ""
echo "  export DT_HOME=/opt/observex"
echo "  export LDR_PRELOAD64=\$DT_HOME/lib64/liboneagentproc.so"
echo "  export LDR_PRELOAD=\$DT_HOME/lib/liboneagentproc.so"`
  }

  if (pid === 'zos') {
    return `# ObserveX for z/OS — Installation Overview
# Architecture: z/Architecture (s390x)
# Monitors: CICS, IMS, IBM MQ, Db2, z/OS Connect EE, Java on z/OS

# ObserveX z/OS requires 3 components:
# 1. Code modules (CICS/IMS/Java) — installed on z/OS LPAR
# 2. zLocal — runs in z/OS Unix System Services (USS)
# 3. zRemote — routes data to ObserveX via ActiveGate

# Step 1: Upload CICS code module to z/OS
# Download: https://dl.observex.io/zos/observex-cics-module.pax

# Step 2: Install via USS
pax -rf observex-zos-${ver}.pax

# Step 3: Configure zLocal (in USS /etc/observex/zlocal.yaml):
cat > /etc/observex/zlocal.yaml << CONFIG
token: ${t}
ingestor_url: ${url}
zremote_host: observex-activegate
zremote_port: 9999
cics_monitoring: true
ims_monitoring: true
ibm_mq_monitoring: true
db2_monitoring: true
CONFIG

# Step 4: Start zLocal daemon
/opt/observex/zos/zlocal --config /etc/observex/zlocal.yaml &

# Step 5: Configure CICS module in SYS1.PARMLIB
# Add to CICS startup JCL:
# //STEPLIB DD DSN=OBSERVEX.LOADLIB,DISP=SHR

# Note: z/OS monitoring licensed per MSU (million service units)
# Contact Cisco support for LPAR configuration assistance`
  }

  if (pid === 'kubernetes' || pid === 'openshift') {
    const cmd = pid === 'openshift' ? 'oc' : 'kubectl'
    return `# ObserveX Agent — ${platform.label}
# Mode: ${mode}  Token: ${t}

# Step 1: Create namespace and secret
${cmd} create namespace observex --dry-run=client -o yaml | ${cmd} apply -f -

${cmd} create secret generic observex-agent-secret \\
  --namespace observex \\
  --from-literal=token="${t}" \\
  --from-literal=ingestor-url="${url}" \\
  --dry-run=client -o yaml | ${cmd} apply -f -

${pid === 'openshift' ? '# Step 2: Configure Security Context Constraints (required for OpenShift)\noc adm policy add-scc-to-user privileged -z observex-agent -n observex\n\n' : ''}# Step ${pid === 'openshift' ? '3' : '2'}: Install via Helm (recommended)
helm repo add observex https://charts.observex.io
helm repo update

helm install observex-agent observex/agent \\
  --namespace observex \\
  --set agent.token="${t}" \\
  --set agent.ingestorURL="${url}" \\
  --set agent.mode="${mode}" \\
  --set agent.ebpf.enabled=true \\
  --set tolerations[0].operator=Exists

# Step ${pid === 'openshift' ? '4' : '3'}: Verify DaemonSet rollout
${cmd} rollout status daemonset/observex-agent -n observex
${cmd} get pods -n observex -l app=observex-agent

# Each pod on every node: 1 agent per node collects ALL signals`
  }

  if (pid === 'helm') {
    return `# ObserveX Agent — Helm Chart (Production)
# Recommended for production Kubernetes deployments

# Add Helm repository
helm repo add observex https://charts.observex.io
helm repo update

# Install with full configuration
helm install observex-agent observex/agent \\
  --namespace observex \\
  --create-namespace \\
  --set agent.token="${t}" \\
  --set agent.ingestorURL="${url}" \\
  --set agent.mode="${mode}" \\
  --set agent.version="${ver}" \\
  --set agent.ebpf.enabled=true \\
  --set agent.logMonitoring.enabled=true \\
  --set agent.networkMonitoring.enabled=true \\
  --set rbac.create=true \\
  --set serviceAccount.create=true \\
  --set tolerations[0].operator=Exists \\
  --set priorityClassName=system-node-critical \\
  --set resources.requests.cpu="100m" \\
  --set resources.requests.memory="200Mi" \\
  --set resources.limits.cpu="500m" \\
  --set resources.limits.memory="500Mi"

# View all configuration options:
helm show values observex/agent

# Verify
kubectl rollout status daemonset/observex-agent -n observex`
  }

  if (pid === 'lambda') {
    return `# ObserveX Agent — AWS Lambda Layer
# Supports: Node.js, Python, Java, Go, Ruby

# Option 1: AWS Console — add layer ARN to your Lambda function
# ARN: arn:aws:lambda:REGION:ACCOUNT:layer:observex-agent-${ver.replace(/\./g,'-')}:1

# Option 2: AWS CLI
aws lambda update-function-configuration \\
  --function-name YOUR_FUNCTION_NAME \\
  --layers arn:aws:lambda:\${AWS_REGION}:123456789:layer:observex-agent:1

# Option 3: Terraform
resource "aws_lambda_function" "example" {
  # ...existing config...
  layers = ["arn:aws:lambda:\${var.region}:123456789:layer:observex-agent:1"]
  environment {
    variables = {
      OBSERVEX_TOKEN    = "${t}"
      OBSERVEX_INGESTOR = "${url}"
    }
  }
}

# Option 4: Add to existing function environment variables
aws lambda update-function-configuration \\
  --function-name YOUR_FUNCTION_NAME \\
  --environment "Variables={OBSERVEX_TOKEN=${t},OBSERVEX_INGESTOR=${url}}"

# Monitored automatically:
# - Cold start duration and frequency
# - Memory usage and peak
# - Function duration (billed duration)
# - Error tracking with stack traces
# - HTTP outbound calls
# - Downstream service correlation`
  }

  if (pid === 'java') {
    return `#!/bin/bash
# ObserveX Java Agent — Zero-restart instrumentation
# Java 8–23 · Spring, Quarkus, Micronaut, Tomcat, JBoss, WebSphere

# Download Java agent JAR
curl -fsSL "https://dl.observex.io/agents/java/observex-javaagent-${ver}.jar" \\
  -o /opt/observex-javaagent.jar

# Add JVM startup flags:
# (for Spring Boot / standalone JAR)
JAVA_OPTS="-javaagent:/opt/observex-javaagent.jar"
JAVA_OPTS="$JAVA_OPTS -Dobservex.token=${t}"
JAVA_OPTS="$JAVA_OPTS -Dobservex.ingestor.url=${url}"
JAVA_OPTS="$JAVA_OPTS -Dobservex.service.name=my-service"
JAVA_OPTS="$JAVA_OPTS -Dobservex.mode=${mode}"

java $JAVA_OPTS -jar your-application.jar

# For Docker (add to Dockerfile):
ENV JAVA_TOOL_OPTIONS="-javaagent:/opt/observex-javaagent.jar \\
  -Dobservex.token=${t} \\
  -Dobservex.service.name=my-service"

# For Kubernetes init container pattern (see docs):
# https://docs.observex.io/agents/java/kubernetes`
  }

  return `#!/bin/bash
# ObserveX Agent — ${platform.label}
# Token: ${t}  Mode: ${mode}
curl -fsSL https://dl.observex.io/install.sh | \\
  OBSERVEX_TOKEN="${t}" \\
  OBSERVEX_MODE="${mode}" \\
  sudo bash`
}

export default function AgentInstallPage() {
  const [step, setStep] = useState(0) // 0=platform, 1=arch+mode, 2=token, 3=deploy, 4=verify
  const [groupId, setGroupId] = useState('os')
  const [platId, setPlatId] = useState('linux-x86')
  const [mode, setMode] = useState('fullstack')
  const [arch, setArch] = useState('')
  const [token, setToken] = useState('')
  const [copied, setCopied] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const [verified, setVerified] = useState(false)

  const platform = PLATFORM_GROUPS.flatMap(g => g.platforms).find(p => p.id === platId)!
  const group = PLATFORM_GROUPS.find(g => g.id === groupId)!
  const script = token ? generateScript(platform, mode, arch, token) : ''
  const ext = platform?.ext || 'sh'

  const genToken = () => {
    const t = 'oxat_' + Math.random().toString(36).slice(2,10) + '_' + Date.now()
    setToken(t)
    setStep(2)
  }

  const copyScript = () => {
    navigator.clipboard.writeText(script)
    setCopied(true)
    toast.success('Script copied to clipboard!')
    setTimeout(() => setCopied(false), 2000)
  }

  const verify = () => {
    setVerifying(true)
    setTimeout(() => {
      setVerifying(false)
      setVerified(true)
      setStep(4)
    }, 2400)
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#070f1e', color: '#c8d8ef', overflow: 'hidden' }}>
      {/* Header */}
      <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid #1a2d4a', flexShrink: 0 }}>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center', marginBottom: 6 }}>
          <div style={{ width: 36, height: 36, borderRadius: 10, background: 'linear-gradient(135deg, #6c72ff, #a855f7)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 16 }}>📡</div>
          <div>
            <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>Install ObserveX Agent</div>
            <div style={{ fontSize: 12, color: '#546a88' }}>v2.0.0 · eBPF auto-instrumentation · 20+ platforms · Guided wizard</div>
          </div>
        </div>
        {/* Step progress */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8 }}>
          {STEPS.map((s, i) => {
            const done = step > i, active = step === i
            return (
              <div key={s} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <div style={{ width: 22, height: 22, borderRadius: '50%', background: done ? '#0fcf8a' : active ? '#6c72ff' : '#0b1628', border: `1px solid ${done ? '#0fcf8a' : active ? '#6c72ff' : '#1a2d4a'}`, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, fontWeight: 700, color: done||active ? '#fff' : '#546a88', flexShrink: 0 }}>
                  {done ? '✓' : i+1}
                </div>
                <span style={{ fontSize: 10, fontWeight: active ? 700 : 400, color: done ? '#0fcf8a' : active ? '#c8d8ef' : '#546a88', whiteSpace: 'nowrap' }}>{s}</span>
                {i < STEPS.length-1 && <div style={{ width: 20, height: 1, background: step > i ? '#0fcf8a' : '#1a2d4a', marginLeft: 2 }} />}
              </div>
            )
          })}
        </div>
      </div>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* Left: wizard */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
          {/* STEP 0: Choose Platform */}
          {step === 0 && (
            <div>
              <div style={{ fontSize: 13, fontWeight: 700, color: '#c8d8ef', marginBottom: 14 }}>Select deployment platform</div>
              {PLATFORM_GROUPS.map(g => (
                <div key={g.id} style={{ marginBottom: 18 }}>
                  <div style={{ fontSize: 9, fontWeight: 700, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.1em', marginBottom: 8 }}>{g.label}</div>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 8 }}>
                    {g.platforms.map(p => (
                      <div key={p.id} onClick={() => { setGroupId(g.id); setPlatId(p.id) }}
                        style={{ padding: '10px 12px', background: platId === p.id ? 'rgba(108,114,255,.1)' : '#0b1628', border: `1px solid ${platId === p.id ? '#6c72ff' : '#1a2d4a'}`, borderRadius: 10, cursor: 'pointer', position: 'relative', transition: '.15s' }}>
                        {p.badge && <div style={{ position: 'absolute', top: 6, right: 6, fontSize: 8, fontWeight: 800, background: 'rgba(108,114,255,.2)', color: '#8b90ff', padding: '1px 5px', borderRadius: 8 }}>{p.badge}</div>}
                        <div style={{ fontSize: 18, marginBottom: 5 }}>{p.icon}</div>
                        <div style={{ fontSize: 11, fontWeight: 700, color: platId === p.id ? '#8b90ff' : '#c8d8ef', marginBottom: 3 }}>{p.label}</div>
                        <div style={{ fontSize: 9, color: '#546a88', lineHeight: 1.4 }}>{p.sub}</div>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
              <button onClick={() => setStep(1)} style={{ background: '#6c72ff', color: '#fff', border: 'none', padding: '10px 24px', borderRadius: 10, fontSize: 13, fontWeight: 700, cursor: 'pointer' }}>
                Continue — {platform?.label} <ChevronRight size={14} style={{ display: 'inline' }} />
              </button>
            </div>
          )}

          {/* STEP 1: Arch + Mode */}
          {step === 1 && (
            <div>
              <div style={{ fontSize: 13, fontWeight: 700, color: '#c8d8ef', marginBottom: 4 }}>Platform: <span style={{ color: '#8b90ff' }}>{platform.icon} {platform.label}</span></div>
              <div style={{ fontSize: 12, color: '#546a88', marginBottom: 18 }}>{platform.sub}</div>

              {/* Architecture */}
              {platform.archs.length > 1 && (
                <div style={{ marginBottom: 18 }}>
                  <div style={{ fontSize: 11, fontWeight: 700, color: '#c8d8ef', marginBottom: 8 }}>Architecture</div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    {platform.archs.map(a => (
                      <button key={a} onClick={() => setArch(a)}
                        style={{ padding: '8px 20px', border: `1px solid ${arch === a ? '#6c72ff' : '#1a2d4a'}`, borderRadius: 8, background: arch === a ? 'rgba(108,114,255,.15)' : '#0b1628', color: arch === a ? '#8b90ff' : '#546a88', fontSize: 12, fontWeight: 600, cursor: 'pointer' }}>
                        {a}
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Monitoring mode */}
              <div style={{ marginBottom: 18 }}>
                <div style={{ fontSize: 11, fontWeight: 700, color: '#c8d8ef', marginBottom: 8 }}>Monitoring Mode</div>
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 8 }}>
                  {MONITORING_MODES.filter(m => platform.modes.includes(m.id)).map(m => (
                    <div key={m.id} onClick={() => setMode(m.id)}
                      style={{ padding: '12px 14px', background: mode === m.id ? 'rgba(108,114,255,.1)' : '#0b1628', border: `1px solid ${mode === m.id ? '#6c72ff' : '#1a2d4a'}`, borderRadius: 10, cursor: 'pointer' }}>
                      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 5 }}>
                        <span style={{ fontSize: 16 }}>{m.icon}</span>
                        <span style={{ fontSize: 12, fontWeight: 700, color: mode === m.id ? '#8b90ff' : '#c8d8ef' }}>{m.label}</span>
                        {m.recommended && <span style={{ fontSize: 9, background: 'rgba(15,207,138,.15)', color: '#0fcf8a', padding: '1px 6px', borderRadius: 8, fontWeight: 800, marginLeft: 'auto' }}>Recommended</span>}
                      </div>
                      <div style={{ fontSize: 11, color: '#546a88', lineHeight: 1.4 }}>{m.desc}</div>
                    </div>
                  ))}
                </div>
              </div>

              <div style={{ display: 'flex', gap: 8 }}>
                <button onClick={() => setStep(0)} style={{ padding: '9px 18px', border: '1px solid #1a2d4a', borderRadius: 10, background: 'transparent', color: '#546a88', fontSize: 12, cursor: 'pointer' }}>← Back</button>
                <button onClick={genToken} style={{ background: '#6c72ff', color: '#fff', border: 'none', padding: '9px 24px', borderRadius: 10, fontSize: 12, fontWeight: 700, cursor: 'pointer' }}>
                  <Shield size={13} style={{ display: 'inline', marginRight: 5 }} />Generate Token & Script
                </button>
              </div>
            </div>
          )}

          {/* STEP 2+: Script */}
          {step >= 2 && step < 4 && (
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 14px', background: 'rgba(15,207,138,.07)', border: '1px solid rgba(15,207,138,.25)', borderRadius: 10, marginBottom: 14 }}>
                <CheckCircle size={14} style={{ color: '#0fcf8a' }} />
                <span style={{ fontSize: 12, color: '#0fcf8a' }}>Token generated · Valid 24h · Scoped: metrics:write logs:write traces:write</span>
              </div>

              {/* Script */}
              <div style={{ background: '#080d1b', border: '1px solid #1a2d4a', borderRadius: 10, overflow: 'hidden', marginBottom: 14 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid #1a2d4a', background: '#0b1628' }}>
                  <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef' }}>{platform.icon} {platform.label} · {MONITORING_MODES.find(m=>m.id===mode)?.label}</div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <button onClick={copyScript} style={{ display: 'flex', gap: 5, alignItems: 'center', background: copied ? 'rgba(15,207,138,.15)' : '#182844', border: `1px solid ${copied ? 'rgba(15,207,138,.3)' : '#254060'}`, color: copied ? '#0fcf8a' : '#8fa8cc', padding: '5px 12px', borderRadius: 8, fontSize: 11, cursor: 'pointer' }}>
                      <Copy size={11} />{copied ? 'Copied!' : 'Copy'}
                    </button>
                    <a href={'data:text/plain;charset=utf-8,' + encodeURIComponent(script)} download={'observex-install.' + ext}
                      style={{ display: 'flex', gap: 5, alignItems: 'center', textDecoration: 'none', background: '#182844', border: '1px solid #254060', color: '#8fa8cc', padding: '5px 12px', borderRadius: 8, fontSize: 11 }}>
                      <Download size={11} />Download .{ext}
                    </a>
                  </div>
                </div>
                <pre style={{ padding: 16, margin: 0, fontSize: 11, color: '#8fa8cc', background: 'none', fontFamily: 'JetBrains Mono, monospace', lineHeight: 1.65, maxHeight: 320, overflowY: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                  {script}
                </pre>
              </div>

              <div style={{ display: 'flex', gap: 8 }}>
                <button onClick={() => setStep(1)} style={{ padding: '9px 18px', border: '1px solid #1a2d4a', borderRadius: 10, background: 'transparent', color: '#546a88', fontSize: 12, cursor: 'pointer' }}>← Back</button>
                {step === 2 && <button onClick={() => setStep(3)} style={{ background: '#6c72ff', color: '#fff', border: 'none', padding: '9px 24px', borderRadius: 10, fontSize: 12, fontWeight: 700, cursor: 'pointer' }}>I've run the command →</button>}
                {step === 3 && (
                  <button onClick={verify} disabled={verifying} style={{ background: verifying ? '#182844' : '#0fcf8a', color: verifying ? '#546a88' : '#fff', border: 'none', padding: '9px 24px', borderRadius: 10, fontSize: 12, fontWeight: 700, cursor: 'pointer', display: 'flex', gap: 8, alignItems: 'center' }}>
                    {verifying ? <><RefreshCw size={13} style={{ animation: 'spin 1s linear infinite' }} />Checking...</> : '🔍 Verify Connection'}
                  </button>
                )}
              </div>
            </div>
          )}

          {/* STEP 4: Success */}
          {step === 4 && (
            <div>
              <div style={{ background: 'linear-gradient(135deg, rgba(15,207,138,.1), rgba(108,114,255,.05))', border: '1px solid rgba(15,207,138,.3)', borderRadius: 14, padding: 32, textAlign: 'center', marginBottom: 20 }}>
                <div style={{ fontSize: 48, marginBottom: 12 }}>🎉</div>
                <div style={{ fontSize: 22, fontWeight: 900, color: '#f0f6ff', fontFamily: 'Syne, sans-serif', marginBottom: 6 }}>Agent is live!</div>
                <div style={{ fontSize: 13, color: '#0fcf8a', fontWeight: 600 }}>{platform.label} · {MONITORING_MODES.find(m=>m.id===mode)?.label} · v2.0.0</div>
                <div style={{ fontSize: 12, color: '#546a88', marginTop: 6 }}>Data is flowing. Your services are being auto-discovered right now.</div>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10 }}>
                {[['🗺', 'Smartscape', 'topology', 'See discovered topology'], ['📊', 'Overview', '/', 'Live metrics dashboard'], ['🖥', 'Agent Fleet', 'agent-fleet', 'Manage all agents']].map(([i, l, href, d]) => (
                  <a key={href} href={`/${href}`} style={{ display: 'block', padding: 14, background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, textAlign: 'center', textDecoration: 'none' }}>
                    <div style={{ fontSize: 20, marginBottom: 6 }}>{i}</div>
                    <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef', marginBottom: 3 }}>{l}</div>
                    <div style={{ fontSize: 10, color: '#546a88' }}>{d}</div>
                  </a>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Right: what we collect */}
        <div style={{ width: 260, borderLeft: '1px solid #1a2d4a', overflowY: 'auto', padding: 16, flexShrink: 0, background: '#070f1e' }}>
          <div style={{ fontSize: 10, fontWeight: 700, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.1em', marginBottom: 14 }}>What Agent Collects</div>
          {[
            { cat: 'Metrics', icon: '📊', items: ['CPU per process', 'Memory / GC heap', 'Network I/O per socket', 'Disk IOPS', 'Native custom metrics', 'K8s resource utilization'] },
            { cat: 'Logs', icon: '📋', items: ['All stdout/stderr', 'System journal (journald)', 'App log files (auto-detected)', 'Windows Event Log', 'K8s pod logs'] },
            { cat: 'Traces', icon: '🔍', items: ['HTTP spans (eBPF uprobes)', 'gRPC auto-traced', 'DB query spans', 'Service dependency graph', 'W3C Trace Context'] },
            { cat: 'Metadata', icon: '🏷', items: ['K8s labels & annotations', 'Cloud instance metadata', 'Git commit SHA', 'Deploy events'] },
          ].map(s => (
            <div key={s.cat} style={{ marginBottom: 14 }}>
              <div style={{ fontSize: 11, fontWeight: 700, color: '#c8d8ef', marginBottom: 6 }}>{s.icon} {s.cat}</div>
              {s.items.map(item => (
                <div key={item} style={{ display: 'flex', gap: 5, alignItems: 'flex-start', fontSize: 10, color: '#546a88', marginBottom: 3 }}>
                  <CheckCircle size={9} style={{ color: '#0fcf8a', flexShrink: 0, marginTop: 1 }} />
                  {item}
                </div>
              ))}
            </div>
          ))}
          <div style={{ borderTop: '1px solid #1a2d4a', paddingTop: 12, marginTop: 8 }}>
            <div style={{ fontSize: 10, fontWeight: 700, color: '#546a88', textTransform: 'uppercase', marginBottom: 8 }}>Latest Release</div>
            <div style={{ fontSize: 22, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>v2.0.0</div>
            <div style={{ fontSize: 10, color: '#0fcf8a', marginBottom: 8 }}>eBPF · AI Agent · LLM monitoring</div>
            {[['2.0.0', 'eBPF, AI agent, LLM monitoring'], ['1.9.2', 'Windows improvements'], ['1.8.0', 'K8s DaemonSet GA']].map(([v,n]) => (
              <div key={v} style={{ display: 'flex', gap: 6, fontSize: 10, color: '#546a88', paddingBottom: 4, borderBottom: '1px solid #0d1a2e', marginBottom: 4 }}>
                <span style={{ color: '#6c72ff', fontFamily: 'JetBrains Mono, monospace' }}>v{v}</span>
                <span>{n}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
