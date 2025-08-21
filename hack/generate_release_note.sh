#!/usr/bin/env bash

set -x

RELEASE=$1
OUTPUT=${2:-release-notes.md}

install_cli() {
  if ! [[ -x "$(command -v release-notes)" ]]; then
    echo "CLI release-notes not found, installing..."
    GO111MODULE=on go install k8s.io/release/cmd/release-notes@latest
  else
    echo "CLI release-notes found, skip installing. If you want to upgrade, run 'GO111MODULE=on go install k8s.io/release/cmd/release-notes@latest'"
  fi
}

generate() {
  FROM_TAG=$1
  TO_TAG=$2
  BRANCH=$3
  FROM_COMMIT=$(git rev-list --no-merges ${FROM_TAG}..${TO_TAG} | tail -1) # exclude the ${FROM_TAG} commit
  TO_COMMIT=$(git rev-parse ${TO_TAG}^{commit})

  echo "Generating release notes for ${FROM_TAG}..${TO_TAG} (${FROM_COMMIT}..${TO_COMMIT}) on branch ${BRANCH}"

  rm -f ${OUTPUT}
  release-notes --repo=kube-egress-gateway \
    --org=Azure \
    --branch=${BRANCH} \
    --start-sha=${FROM_COMMIT} \
    --end-sha=${TO_COMMIT} \
    --markdown-links=true \
    --required-author='' \
    --output=${OUTPUT}

  read -r -d '' HEAD <<EOF
Full Changelog: [${FROM_TAG}..${TO_TAG}](https://github.com/Azure/kube-egress-gateway/compare/${FROM_TAG}...${TO_TAG})
EOF

  echo -e "${HEAD}\n\n$(cat ${OUTPUT})" > ${OUTPUT}

}

help() {
  echo "Usage: $0 <release> [output]"
  echo "  release: release tag, e.g. v1.21.0."
  echo "  output: output file, default to release-notes.md."
  echo "  GITHUB_TOKEN: The GitHub token to use for API requests. (required environment variable)"
  echo "Example: $0 v1.21.0"
  echo "Example: $0 v1.21.0 release-notes.md"
}

main() {
  if [[ -z "${RELEASE}" || -z "${GITHUB_TOKEN}" ]]; then
    help
    exit 1
  fi

  install_cli

  VERSION="${RELEASE#[vV]}"
  VERSION_MAJOR="${VERSION%%\.*}"
  VERSION_MINOR="${VERSION#*.}"
  VERSION_MINOR="${VERSION_MINOR%.*}"
  VERSION_PATCH="${VERSION##*.}"

  # when release a new minor version, generate release note from the last minor version
  # example:
  #   when release v1.21.0, generate release note from v1.20.0 (v1.20.0..v1.21.0)
  #   when release v1.21.3, generate release note from v1.21.2 (v1.21.2..v1.21.3)
  if [[ "${VERSION_PATCH}" = "0" ]]; then
    START_TAG=v${VERSION_MAJOR}.$((VERSION_MINOR-1)).0
  else
    START_TAG=v${VERSION_MAJOR}.${VERSION_MINOR}.$((VERSION_PATCH-1))
  fi
  END_TAG=${RELEASE}

  generate ${START_TAG} ${END_TAG} main 
}

main
