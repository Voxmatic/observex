{{/*
observex/_helpers.tpl
Shared template helpers. Included by all other templates.
*/}}

{{/* ── Name helpers ─────────────────────────────────────────────────────────── */}}

{{- define "observex.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "observex.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "observex.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* Component name: <release>-<chart>-<component> */}}
{{- define "observex.componentName" -}}
{{- printf "%s-%s" (include "observex.fullname" .) .component | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* ── Labels ──────────────────────────────────────────────────────────────── */}}

{{- define "observex.labels" -}}
helm.sh/chart: {{ include "observex.chart" . }}
{{ include "observex.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: observex
{{- end }}

{{- define "observex.selectorLabels" -}}
app.kubernetes.io/name: {{ include "observex.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "observex.componentLabels" -}}
{{ include "observex.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "observex.componentSelectorLabels" -}}
{{ include "observex.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/* ── ServiceAccount ──────────────────────────────────────────────────────── */}}

{{- define "observex.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "observex.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* ── Image ───────────────────────────────────────────────────────────────── */}}

{{- define "observex.image" -}}
{{- $registry := .Values.global.imageRegistry | default "" -}}
{{- $repository := .image.repository -}}
{{- $tag := .image.tag | default $.Chart.AppVersion -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end }}

{{/* ── Secret key reference helper ────────────────────────────────────────── */}}
{{/*
  Usage: {{ include "observex.secretRef" (dict "secretName" (include "observex.secretName" .) "key" "jwt-secret") }}
  Returns a valueFrom.secretKeyRef block, pointing to either:
    a) existingSecret if set, or
    b) the auto-generated secret named <fullname>-secrets
*/}}
{{- define "observex.secretRef" -}}
{{- $secretName := .secretName -}}
{{- $key := .key -}}
valueFrom:
  secretKeyRef:
    name: {{ $secretName }}
    key: {{ $key }}
{{- end }}

{{/* The name of the secrets object managed by this chart */}}
{{- define "observex.secretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{ .Values.secrets.existingSecret }}
{{- else -}}
{{ include "observex.fullname" . }}-secrets
{{- end -}}
{{- end }}

{{/* ── Storage URLs ─────────────────────────────────────────────────────────── */}}
{{/* These helpers return the correct URL whether the sub-chart is enabled
     (use in-cluster service name) or external (use externalUrl). */}}

{{- define "observex.queryEngineUrl" -}}
http://{{ include "observex.fullname" . }}-query-engine:9090
{{- end }}

{{- define "observex.lokiUrl" -}}
{{- if .Values.loki.externalUrl -}}
{{ .Values.loki.externalUrl }}
{{- else -}}
http://{{ include "observex.fullname" . }}-loki:3100
{{- end -}}
{{- end }}

{{- define "observex.tempoUrl" -}}
{{- if .Values.tempo.externalUrl -}}
{{ .Values.tempo.externalUrl }}
{{- else -}}
http://{{ include "observex.fullname" . }}-tempo:3200
{{- end -}}
{{- end }}

{{- define "observex.neo4jUrl" -}}
{{- if .Values.neo4j.externalUrl -}}
{{ .Values.neo4j.externalUrl }}
{{- else -}}
bolt://{{ include "observex.fullname" . }}-neo4j:7687
{{- end -}}
{{- end }}

{{- define "observex.postgresHost" -}}
{{- if .Values.postgresql.externalHost -}}
{{ .Values.postgresql.externalHost }}
{{- else -}}
{{ include "observex.fullname" . }}-postgresql
{{- end -}}
{{- end }}

{{- define "observex.redisHost" -}}
{{- if .Values.redis.externalHost -}}
{{ .Values.redis.externalHost }}
{{- else -}}
{{ include "observex.fullname" . }}-redis-master
{{- end -}}
{{- end }}

{{- define "observex.clickhouseHost" -}}
{{- if .Values.clickhouse.externalHost -}}
{{ .Values.clickhouse.externalHost }}
{{- else -}}
{{ include "observex.fullname" . }}-clickhouse
{{- end -}}
{{- end }}

{{/* ── Service URLs (internal) ─────────────────────────────────────────────── */}}

{{- define "observex.ingestorUrl" -}}
http://{{ include "observex.fullname" . }}-ingestor:{{ .Values.ingestor.service.port }}
{{- end }}

{{- define "observex.processorUrl" -}}
http://{{ include "observex.fullname" . }}-processor:{{ .Values.processor.service.port }}
{{- end }}

{{- define "observex.aiAgentUrl" -}}
http://{{ include "observex.fullname" . }}-ai-agent:{{ .Values.aiAgent.service.port }}
{{- end }}

{{- define "observex.topologyUrl" -}}
http://{{ include "observex.fullname" . }}-topology:{{ .Values.topology.service.port }}
{{- end }}

{{- define "observex.profilingUrl" -}}
http://{{ include "observex.fullname" . }}-profiling:{{ .Values.profiling.service.port }}
{{- end }}

{{- define "observex.trivyScannerUrl" -}}
http://{{ include "observex.fullname" . }}-trivy-scanner:8080
{{- end }}

{{/* ── Common pod spec fragments ───────────────────────────────────────────── */}}

{{- define "observex.imagePullSecrets" -}}
{{- with .Values.global.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
