{{/*
Determine gateway-controller-manager image
*/}}
{{- define "image.gatewayControllerManager" -}}
    {{- if hasKey .Values.gatewayControllerManager "imageTag" -}}
        {{- printf "%s/%s:%s" .Values.gatewayControllerManager.imageRepository .Values.gatewayControllerManager.imageName .Values.gatewayControllerManager.imageTag -}}
    {{- else -}}
        {{- printf "%s/%s:%s" .Values.common.imageRepository .Values.gatewayControllerManager.imageName .Values.common.imageTag -}}
    {{- end -}}
{{- end -}}

{{/*
Determine gateway-daemon-manager image
*/}}
{{- define "image.gatewayDaemonManager" -}}
    {{- if hasKey .Values.gatewayDaemonManager "imageTag" -}}
        {{- printf "%s/%s:%s" .Values.gatewayDaemonManager.imageRepository .Values.gatewayDaemonManager.imageName .Values.gatewayDaemonManager.imageTag -}}
    {{- else -}}
        {{- printf "%s/%s:%s" .Values.common.imageRepository .Values.gatewayDaemonManager.imageName .Values.common.imageTag -}}
    {{- end -}}
{{- end -}}

{{/*
Determine gateway-CNI-manager image
*/}}
{{- define "image.gatewayCNIManager" -}}
    {{- if hasKey .Values.gatewayCNIManager "imageTag" -}}
        {{- printf "%s/%s:%s" .Values.gatewayCNIManager.imageRepository .Values.gatewayCNIManager.imageName .Values.gatewayCNIManager.imageTag -}}
    {{- else -}}
        {{- printf "%s/%s:%s" .Values.common.imageRepository .Values.gatewayCNIManager.imageName .Values.common.imageTag -}}
    {{- end -}}
{{- end -}}

{{/*
Determine gateway-CNI image
*/}}
{{- define "image.gatewayCNI" -}}
    {{- if hasKey .Values.gatewayCNI "imageTag" -}}
        {{- printf "%s/%s:%s" .Values.gatewayCNI.imageRepository .Values.gatewayCNI.imageName .Values.gatewayCNI.imageTag -}}
    {{- else -}}
        {{- printf "%s/%s:%s" .Values.common.imageRepository .Values.gatewayCNI.imageName .Values.common.imageTag -}}
    {{- end -}}
{{- end -}}

{{/*
Determine gateway-CNI-Ipam image
*/}}
{{- define "image.gatewayCNIIpam" -}}
    {{- if hasKey .Values.gatewayCNIIpam "imageTag" -}}
        {{- printf "%s/%s:%s" .Values.gatewayCNIIpam.imageRepository .Values.gatewayCNIIpam.imageName .Values.gatewayCNIIpam.imageTag -}}
    {{- else -}}
        {{- printf "%s/%s:%s" .Values.common.imageRepository .Values.gatewayCNIIpam.imageName .Values.common.imageTag -}}
    {{- end -}}
{{- end -}}
