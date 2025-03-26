#!/usr/bin/env bash
set -euo pipefail

RELEASE=$1
OUTPUT=${2:-release-notes.md}


help() {
  echo "Usage: $0 <release> [output] [update_site]"
  echo "  release: release tag, e.g. v1.21.0."
  echo "  output: output file, default to release-notes.md."
  echo "Example: $0 v1.21.0"
  echo "Example: $0 v1.21.0 release-notes.md"
}

main() {
  if [[ -z "${RELEASE}" ]]; then
    help
    exit 1
  fi

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

  cat <<EOF > ${OUTPUT}
Full Changelog: [${START_TAG}..${END_TAG}](https://github.com/kubernetes-sigs/cloud-provider-azure/compare/${START_TAG}...${END_TAG})
EOF
  
}

main