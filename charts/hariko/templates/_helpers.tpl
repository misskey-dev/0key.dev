{{- define "hariko.name" -}}
hariko-{{- default .Values.name | replace "." "-" -}}
{{- end -}}
