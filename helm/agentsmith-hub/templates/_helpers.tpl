{{/*
Expand the name of the chart.
*/}}
{{- define "agentsmith-hub.name" -}}
{{- default .Chart.Name .Values.global.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "agentsmith-hub.fullname" -}}
{{- if .Values.global.fullnameOverride }}
{{- .Values.global.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.global.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "agentsmith-hub.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "agentsmith-hub.labels" -}}
helm.sh/chart: {{ include "agentsmith-hub.chart" . }}
{{ include "agentsmith-hub.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "agentsmith-hub.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentsmith-hub.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "agentsmith-hub.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "agentsmith-hub.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the leader deployment
*/}}
{{- define "agentsmith-hub.leader.fullname" -}}
{{- printf "%s-leader" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the follower deployment
*/}}
{{- define "agentsmith-hub.follower.fullname" -}}
{{- printf "%s-follower" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the frontend deployment
*/}}
{{- define "agentsmith-hub.frontend.fullname" -}}
{{- printf "%s-frontend" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the redis deployment
*/}}
{{- define "agentsmith-hub.redis.fullname" -}}
{{- printf "%s-redis" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Leader selector labels
*/}}
{{- define "agentsmith-hub.leader.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentsmith-hub.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: leader
{{- end }}

{{/*
Follower selector labels
*/}}
{{- define "agentsmith-hub.follower.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentsmith-hub.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: follower
{{- end }}

{{/*
Frontend selector labels
*/}}
{{- define "agentsmith-hub.frontend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentsmith-hub.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: frontend
{{- end }}

{{/*
Redis selector labels
*/}}
{{- define "agentsmith-hub.redis.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentsmith-hub.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: redis
{{- end }}

{{/*
Leader labels
*/}}
{{- define "agentsmith-hub.leader.labels" -}}
{{ include "agentsmith-hub.labels" . }}
{{ include "agentsmith-hub.leader.selectorLabels" . }}
{{- end }}

{{/*
Follower labels
*/}}
{{- define "agentsmith-hub.follower.labels" -}}
{{ include "agentsmith-hub.labels" . }}
{{ include "agentsmith-hub.follower.selectorLabels" . }}
{{- end }}

{{/*
Frontend labels
*/}}
{{- define "agentsmith-hub.frontend.labels" -}}
{{ include "agentsmith-hub.labels" . }}
{{ include "agentsmith-hub.frontend.selectorLabels" . }}
{{- end }}

{{/*
Redis labels
*/}}
{{- define "agentsmith-hub.redis.labels" -}}
{{ include "agentsmith-hub.labels" . }}
{{ include "agentsmith-hub.redis.selectorLabels" . }}
{{- end }}

{{/*
Get the image name
*/}}
{{- define "agentsmith-hub.image" -}}
{{- $registryName := .Values.global.imageRegistry | default .Values.image.registry -}}
{{- $repositoryName := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion | toString -}}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- end }}

{{/*
Get the frontend image name
*/}}
{{- define "agentsmith-hub.frontend.image" -}}
{{- $registryName := .Values.global.imageRegistry | default .Values.frontend.image.registry -}}
{{- $repositoryName := .Values.frontend.image.repository -}}
{{- $tag := .Values.frontend.image.tag | default .Chart.AppVersion | toString -}}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- end }}

{{/*
Get the redis image name
*/}}
{{- define "agentsmith-hub.redis.image" -}}
{{- $registryName := .Values.global.imageRegistry | default .Values.redis.image.registry -}}
{{- $repositoryName := .Values.redis.image.repository -}}
{{- $tag := .Values.redis.image.tag | toString -}}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- end }}

{{/*
Create the name of the config volume
*/}}
{{- define "agentsmith-hub.configVolumeName" -}}
{{- printf "%s-config" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the data volume
*/}}
{{- define "agentsmith-hub.dataVolumeName" -}}
{{- printf "%s-data" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the logs volume
*/}}
{{- define "agentsmith-hub.logsVolumeName" -}}
{{- printf "%s-logs" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the redis data volume
*/}}
{{- define "agentsmith-hub.redis.dataVolumeName" -}}
{{- printf "%s-redis-data" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the redis config volume
*/}}
{{- define "agentsmith-hub.redis.configVolumeName" -}}
{{- printf "%s-redis-config" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the mcp config volume
*/}}
{{- define "agentsmith-hub.mcpConfigVolumeName" -}}
{{- printf "%s-mcp-config" (include "agentsmith-hub.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }} 