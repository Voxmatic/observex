{{/*
_f61_helpers.tpl — helpers for F6.1 TLS certificate detection (G-1: a
dedicated synthetic probe service, one Deployment per vantage). Everything
here is rendered only when .Values.f61.enabled is true.
*/}}

{{- define "observex.f61.intakeServiceName" -}}
{{ printf "%s-processor-probe-intake" (include "observex.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
The name probes dial and verify in the listener certificate:
<intake Service>.<release namespace>.svc. Pod DNS resolves it through the
search path Kubernetes configures, so the cluster domain is never named. The
listener certificate's subjectAltName must contain exactly this DNS name; the
intake Service carries it in the observex.io/tls-server-name annotation.
*/}}
{{- define "observex.f61.intakeTLSName" -}}
{{ include "observex.f61.intakeServiceName" . }}.{{ .Release.Namespace }}.svc
{{- end }}

{{/* The processor probe origin the probes use. */}}
{{- define "observex.f61.processorUrl" -}}
{{- if .Values.f61.probe.processorUrl -}}
{{ .Values.f61.probe.processorUrl }}
{{- else if .Values.f61.intake.insecurePlaintext -}}
http://{{ include "observex.f61.intakeTLSName" . }}:{{ .Values.f61.intake.port }}
{{- else -}}
https://{{ include "observex.f61.intakeTLSName" . }}:{{ .Values.f61.intake.port }}
{{- end -}}
{{- end }}

{{/* Fail early on configurations that would leave F6.1 unusable or unsafe. */}}
{{- define "observex.f61.validate" -}}
{{- if .Values.f61.enabled -}}
{{- if not .Values.f61.probeKeySecret.name -}}
{{- fail "f61.probeKeySecret.name is required when f61.enabled is true" -}}
{{- end -}}
{{- if and (not .Values.f61.intake.tlsSecret) (not .Values.f61.intake.insecurePlaintext) -}}
{{- fail "f61.enabled requires f61.intake.tlsSecret (or f61.intake.insecurePlaintext=true for development only)" -}}
{{- end -}}
{{- $seen := dict -}}
{{- range .Values.f61.vantages -}}
{{- if not (regexMatch "^[a-z0-9]([-a-z0-9]{0,38}[a-z0-9])?$" (toString .name)) -}}
{{- fail (printf "f61.vantages: name %q must be a DNS-1123 label of at most 40 characters" (toString .name)) -}}
{{- end -}}
{{- if hasKey $seen .name -}}
{{- fail (printf "f61.vantages: duplicate name %q" .name) -}}
{{- end -}}
{{- $_ := set $seen .name true -}}
{{- if not .credentialSecret -}}
{{- fail (printf "f61.vantages[%s]: credentialSecret is required" .name) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/* The PostgreSQL DSN env entries for F6.1 (processor and migration Job). */}}
{{- define "observex.f61.dsnEnv" -}}
{{- $f := .Values.f61 -}}
{{- if $f.database.dsnSecret.name }}
- name: OBSERVEX_F61_POSTGRES_DSN
  valueFrom:
    secretKeyRef:
      name: {{ $f.database.dsnSecret.name }}
      key: {{ $f.database.dsnSecret.key }}
{{- else }}
{{- $port := "5432" -}}
{{- if .Values.postgresql.externalHost }}{{- $port = toString .Values.postgresql.externalPort -}}{{- end }}
- name: F61_POSTGRES_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "observex.secretName" . }}
      key: postgres-password
- name: OBSERVEX_F61_POSTGRES_DSN
  value: {{ printf "postgres://%s:$(F61_POSTGRES_PASSWORD)@%s:%s/%s?sslmode=%s" (.Values.postgresql.auth.username | default "observex") (include "observex.postgresHost" .) $port (.Values.postgresql.auth.database | default "observex") $f.database.sslMode | quote }}
{{- end }}
{{- end }}

{{- define "observex.f61.probeKeyPath" -}}
/var/run/secrets/observex-f61/probe-key/key
{{- end }}
