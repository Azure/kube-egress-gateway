#!/bin/bash
set -euo pipefail

# Check if required variables are present
: "${AZURE_SUBSCRIPTION_ID:?Environment variable empty or not defined.}"
: "${AZURE_TENANT_ID:?Environment variable empty or not defined.}"
: "${AZURE_CLIENT_ID:?Environment variable empty or not defined.}"
: "${AZURE_CLIENT_SECRET:?Environment variable empty or not defined.}"

# Check if collecting log is needed
if [[ "${COLLECT_LOG}" == "true" ]]; then
    if [[ -z "${LOG_DIR}" ]]; then
        echo "LOG_DIR is not set"
        exit 1
    fi
fi

collect_pod_specs() {
    echo "Describing kube-egress-gateway pods..."
    kubectl get pod -A -l app=kube-egress-gateway -o json | jq -r '.items[] | .metadata.namespace + " " + .metadata.name' | while read -r NS POD; do
        mkdir -p "${LOG_DIR}/pods-spec/${NS}"
        kubectl describe pod ${POD} -n ${NS} > "${LOG_DIR}/pods-spec/${NS}/${POD}" || echo "Cannot describe pod ${NS}/${POD}"
    done
}

collect_pod_logs() {
    echo "Collecting kube-egress-gateway pods logs..."
    kubectl get pod -A -l app=kube-egress-gateway -o json | jq -r '.items[] | .metadata.namespace + " " + .metadata.name' | while read -r NS POD; do
        mkdir -p "${LOG_DIR}/pods-log/${NS}"
        kubectl logs ${POD} -n ${NS} > "${LOG_DIR}/pods-log/${NS}/${POD}.log" || echo "Cannot collect log from pod ${NS}/${POD}"
    done
}

collect_node_logs() {
    echo "Collecting logs of all nodes"
    mkdir -p "${LOG_DIR}/nodes-log"
    LOG_IMAGE="nginx"
    # TODO: switch to kubelet node log API once it is supported 
    # https://kubernetes.io/docs/concepts/cluster-administration/system-logs/#log-query
    kubectl get node | grep "aks-" | awk '{printf("%s\n",$1)}' | while read -r NODE; do
        NODE_LOG_DIR="${LOG_DIR}/nodes-log/${NODE}"
        mkdir -p "${NODE_LOG_DIR}"
        kubectl debug node/${NODE} -it --image=${LOG_IMAGE} -- cat /host/var/log/syslog  > "${NODE_LOG_DIR}/syslog" || echo "Cannot collect syslog.log from node ${NODE}"
        kubectl debug node/${NODE} -it --image=${LOG_IMAGE} -- cat /host/var/log/kern.log  > "${NODE_LOG_DIR}/kern.log" || echo "Cannot collect kern.log from node ${NODE}"
        kubectl debug node/${NODE} -it --image=${LOG_IMAGE} -- cat /host/var/log/messages  > "${NODE_LOG_DIR}/messages" || echo "Cannot collect messages from node ${NODE}"
        kubectl debug node/${NODE} -it --image=${LOG_IMAGE} -- cat /host/etc/cni/net.d/01-egressgateway.conflist  > "${NODE_LOG_DIR}/01-egressgateway.conflist" || echo "Cannot collect cni conflist from node ${NODE}"
    done
}

cleanup() {
    echo "Listing nodes..."
    kubectl get node -owide || echo "Unable to get nodes"
    echo "Listing pods..."
    kubectl get pod --all-namespaces=true -owide || echo "Unable to get pods"

    if [[ ${COLLECT_LOG} != "true" ]]; then
        return
    fi
    echo "Collecting logs..."
    collect_pod_specs
    collect_pod_logs
    collect_node_logs 
}
trap cleanup EXIT

REPO_ROOT=$(git rev-parse --show-toplevel)

# Run e2e tests
go test ${REPO_ROOT}/e2e/ -v --timeout 30m