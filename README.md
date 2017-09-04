phabricator-matrix-bridge
===

bridge live feed from phabricator to matrix room(s)

[![Build Status](https://travis-ci.org/epiphyte/phabricator-matrix-bridge.svg?branch=master)](https://travis-ci.org/epiphyte/phabricator-matrix-bridge)

# install

setup the epiphyte [repository](https://github.com/epiphyte/repository)

install
```
pacman -S phab-http
```

services
```
systemctl enable phab-http
```

for the phab-http hook - in phabricator (cli) you need to set the http-hook to include a binding localhost port 8080
```
./bin/config set feed.http-hooks ['http://localhost:8080/']
```

---

# phab-http

support for enabling feeding out of phabricator into a synapse room

flow:
1. Phabricator event occurs
2. Daemon is trigger and fires the feed.http-hooks
3. Data is proxied into the phab-http service
4. The 'storyText' is relayed (via matrix curl api) to synapse


## configuration

set the environment vars required in
```
/etc/epiphyte.d/phab-http.conf
```

