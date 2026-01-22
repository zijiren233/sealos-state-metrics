#!/bin/bash

HELM_OPTS=${HELM_OPTS:-""}

helm upgrade -i state-metrics \
    -n devbox-system --create-namespace \
    ./charts/state-metrics \
    --set 'tolerations[0].operator=Exists' \
    ${HELM_OPTS}
