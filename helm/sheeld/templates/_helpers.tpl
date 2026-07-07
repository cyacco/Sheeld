{{/*
Expand the name of the chart.
*/}}
{{- define "sheeld.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "sheeld.fullname" -}}
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
Common labels
*/}}
{{- define "sheeld.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Selector labels for api
*/}}
{{- define "sheeld.api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sheeld.name" . }}-api
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for web
*/}}
{{- define "sheeld.web.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sheeld.name" . }}-web
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for the data plane
*/}}
{{- define "sheeld.dataplane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sheeld.name" . }}-dataplane
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for the control-plane database
*/}}
{{- define "sheeld.cpPostgres.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sheeld.name" . }}-cp-postgres
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for the data-plane database
*/}}
{{- define "sheeld.dpPostgres.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sheeld.name" . }}-dp-postgres
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Control-plane database URL: explicit override, else the bundled cp-postgres.
*/}}
{{- define "sheeld.databaseURL" -}}
{{- if .Values.secrets.databaseURL -}}
{{- .Values.secrets.databaseURL -}}
{{- else -}}
{{- printf "postgres://sheeld:%s@%s-cp-postgres:5432/sheeld?sslmode=disable" .Values.secrets.postgresPassword (include "sheeld.fullname" .) -}}
{{- end -}}
{{- end }}

{{/*
Data-plane database URL: explicit override, else the bundled dp-postgres.
*/}}
{{- define "sheeld.dpDatabaseURL" -}}
{{- if .Values.secrets.dpDatabaseURL -}}
{{- .Values.secrets.dpDatabaseURL -}}
{{- else -}}
{{- printf "postgres://sheeld:%s@%s-dp-postgres:5432/sheeld?sslmode=disable" .Values.secrets.postgresPassword (include "sheeld.fullname" .) -}}
{{- end -}}
{{- end }}

{{/*
Service account name
*/}}
{{- define "sheeld.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "sheeld.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
