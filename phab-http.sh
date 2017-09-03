#!/bin/bash
VARS=/etc/epiphyte.d/phab-http.conf
source $VARS
for v in $(cat $VARS | grep -v "^#" | cut -d "=" -f 1); do
    export $v
done
phab-http
