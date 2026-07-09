{{/*
Expand the name of the chart.
*/}}
{{- define "pulse.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
Truncate at 63 chars because Kubernetes name fields are limited to 63 characters.
*/}}
{{- define "pulse.fullname" -}}
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

{{/*
Create chart label value.
*/}}
{{- define "pulse.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "pulse.labels" -}}
helm.sh/chart: {{ include "pulse.chart" . }}
{{ include "pulse.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "pulse.selectorLabels" -}}
app.kubernetes.io/name: {{ include "pulse.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use.
*/}}
{{- define "pulse.serviceAccountName" -}}
{{- if .Values.pulse.serviceAccount.create }}
{{- default (include "pulse.fullname" .) .Values.pulse.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.pulse.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
ClickHouse DSN — auto-computed when bundled; override with externalDSN.
When clickhouse.auth.existingSecret is set the DSN comes from the Secret
(key: PULSE_CLICKHOUSE_DSN) and this helper returns empty so the deployment
uses the secretKeyRef path instead of an inline plaintext value.
*/}}
{{- define "pulse.clickhouseDSN" -}}
{{- if .Values.clickhouse.enabled }}
{{- if .Values.clickhouse.auth.existingSecret }}
{{- /* DSN injected from Secret — return empty to signal secretKeyRef path */ -}}
{{- "" -}}
{{- else }}
{{- printf "clickhouse://%s-clickhouse:9000/%s" (include "pulse.fullname" .) .Values.pulse.clickhouse.database }}
{{- end }}
{{- else }}
{{- .Values.clickhouse.externalDSN }}
{{- end }}
{{- end }}

{{/*
ClickHouse Secret name for auth (user/password/DSN).
Returns empty when auth is not configured.
*/}}
{{- define "pulse.clickhouseAuthSecret" -}}
{{- .Values.clickhouse.auth.existingSecret }}
{{- end }}

{{/*
Meta DSN — SQLite on PVC or Postgres when postgres.enabled.
*/}}
{{- define "pulse.metaDSN" -}}
{{- if .Values.postgres.enabled }}
{{- /* Postgres DSN is injected from secret via PULSE_POSTGRES_DSN env var; empty here signals use-secret. */ -}}
{{- "" }}
{{- else if .Values.pulse.meta.dsn }}
{{- .Values.pulse.meta.dsn }}
{{- else }}
{{- "/var/lib/pulse/pulse_meta.db" }}
{{- end }}
{{- end }}
