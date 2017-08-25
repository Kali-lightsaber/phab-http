#!/bin/bash
VARS=/etc/epiphyte.d/phab-http.conf
source $VARS
for v in $(cat $VARS | grep -v "^#" | cut -d "=" -f 1); do
    export $v
done
PATH_TO="/opt/epiphyte/phab-http"
HOOK=${PATH_TO}/phab-http
TMP_HOOK=${PATH_TO}/.phab-http
if [ -e $TMP_HOOK ]; then
    mv $TMP_HOOK $HOOK
fi

$HOOK
