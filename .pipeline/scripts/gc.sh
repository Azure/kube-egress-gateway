#!/bin/bash
set -euo pipefail

CURRENT_DATE="$(date +%s)"

az login --service-principal -u "${AZURE_CLIENT_ID}" -p "${AZURE_CLIENT_SECRET}" --tenant "${AZURE_TENANT_ID}"
az group list --tag usage=pod-egress-e2e | jq -r '.[].name' | awk '{print $1}' | while read -r RESOURCE_GROUP; do
  RG_DATE="$(az group show -g ${RESOURCE_GROUP} | jq -r '.tags.creation_date')"
  RG_DATE_TOSEC="$(date --date="${RG_DATE}" +%s)"
  DATE_DIFF="$(expr ${CURRENT_DATE} - ${RG_DATE_TOSEC})"
  # GC clusters older than 1 day
  if (( "${DATE_DIFF}" > 86400 )); then
    echo "Deleting resource group: ${RESOURCE_GROUP}"
    az group delete --resource-group "${RESOURCE_GROUP}" --yes --no-wait
  fi
done
