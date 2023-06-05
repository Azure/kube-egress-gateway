#!/usr/bin/env bash
#
# Run kube-egress-gateway unit tests.
# 
set -e

# switch into the repo root directory
GIT_ROOT=$(git rev-parse --show-toplevel)
cd $GIT_ROOT

echo "Running unit tests"

mkdir -p /tmp/cni-rootless
declare -a pkg_need_root=("github.com/Azure/kube-egress-gateway/cmd/kube-egress-cni" "github.com/Azure/kube-egress-gateway/cmd/kube-egress-cni-ipam")

PKG=${PKG:-$(go list ./... | xargs echo)}

for t in ${PKG}; do
    if [[ "${pkg_need_root[*]}"  == *"${t}"* ]];
    then 
        bash -c "export XDG_RUNTIME_DIR=/tmp/cni-rootless; unshare -rmn go test ${t} -covermode set"
    elif [[ "${t}" != *"e2e"* ]];
    then
        go test ${t} -covermode set
    fi
done

rm -rf /tmp/cni-rootless
