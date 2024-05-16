#!/usr/bin/env bash

max_attempts="${1:-120}"
attempts=0

while [ $attempts -le "$max_attempts" ]; do
  if [[ $(kubectl get deployment -n capi-k3k-system capi-k3k-controller-manager \
    -o jsonpath='{.status.conditions[?(@.type=="Available")].status}') == 'True' ]]; then
    exit 0
  else
    echo $(date) "Waiting for the CAPI deployment to be ready..."
    sleep 5
    ((attempts++))
  fi
done
echo Timeout
exit 1
