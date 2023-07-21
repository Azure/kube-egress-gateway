#!/usr/bin/env bash
#
# Run kube-egress-gateway unit tests.
# 
set -e

# switch into the repo root directory
GIT_ROOT=$(git rev-parse --show-toplevel)
pushd $GIT_ROOT

echo "Running unit tests"

rm -rf ./testcoverage
mkdir -p ./testcoverage

mkdir -p /tmp/cni-rootless
declare -a pkg_need_root=("github.com/Azure/kube-egress-gateway/cmd/kube-egress-cni" "github.com/Azure/kube-egress-gateway/cmd/kube-egress-cni-ipam")

PKG=${PKG:-$(go list ./... | xargs echo)}

for t in ${PKG}; do
    if [[ "${pkg_need_root[*]}"  == *"${t}"* ]];
    then 
        bash -c "export XDG_RUNTIME_DIR=/tmp/cni-rootless; unshare -rmn go test -cover ${t} -args -test.gocoverdir=${PWD}/testcoverage"
    elif [[ "${t}" != *"e2e"* ]];
    then
        go test -cover ${t} -args -test.gocoverdir="${PWD}/testcoverage"
    fi
done

go tool covdata textfmt -i=${PWD}/testcoverage -o ./profile.cov.tmp
cat ./profile.cov.tmp | grep -v "zz_generated.deepcopy.go" > ./profile.cov
go tool cover -func ./profile.cov

rm -rf /tmp/cni-rootless
rm -rf ./testcoverage
rm ./profile.cov.tmp
popd
