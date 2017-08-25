#!/bin/bash
source /etc/environment

_last() {
	dates=$(date +%Y-%m-%d)
    for i in $(seq 1 30); do
        dates="$dates|"$(date -d "$i days ago" +%Y-%m-%d)
    done
    LOGS=/var/log/phab-http.
    for p in $(cat $LOGS* | grep "^PHID-USER" | cut -d " " -f 1 | sort | uniq); do
        for f in $(ls $LOGS* | grep -E $dates | sort -r); do
            cat $f | grep "<a" | grep -q ^$p 
            if [ $? -ne 1 ]; then
                user=$(cat $f | grep "<a" | grep ^$p| sed "s/.*\/p\///g;s/\/.*//g" | sort | uniq)
		echo "| "$(basename $f | cut -d "." -f 2)" | "[@$user]\(/p/$user\)" |"
                break
            fi
        done
    done
}

_content(){
    echo "
> this page is maintained by a bot
> do _NOT_ edit it here

"
    echo " | date | user |"
    echo " | ---  | ---  |"
    _last | sort -r
}

_lastseen() {
    content=$(_content)
    content=$(python -c "import urllib.parse
print(urllib.parse.quote(\"\"\"$content\"\"\"))")
    echo $content
    curl $PHAB_HOST/api/phriction.edit \
        -d api.token=$SYNAPSE_PHAB_TOKEN \
        -d slug=meta/reports/lastseen \
        -d title=Last%20Seen \
        -d content=$content
}

_lastseen
