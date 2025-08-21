#!/bin/bash
# 
# Update kube-egress-gateway helm chart.
#

set -o nounset
set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

helm package ${REPO_ROOT}/helm/kube-egress-gateway -d ${REPO_ROOT}/helm/repo/new
helm repo index ${REPO_ROOT}/helm/repo/new
helm repo index --merge ${REPO_ROOT}/helm/repo/index.yaml ${REPO_ROOT}/helm/repo/new
mv ${REPO_ROOT}/helm/repo/new/index.yaml ${REPO_ROOT}/helm/repo/index.yaml
mv ${REPO_ROOT}/helm/repo/new/*.tgz ${REPO_ROOT}/helm/repo
rm -r ${REPO_ROOT}/helm/repo/new