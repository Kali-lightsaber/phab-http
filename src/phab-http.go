/*
 * Copyright 2017
 * MIT License
 * Provides a simple hook endpoint for phabricator to call
 * and proxy messages to a matrix/synapse server
 */
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/epiphyte/goutils"
	"html"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// Posting messages to rooms
	MatrixPost = "%s/_matrix/client/r0/rooms/%s/send/m.room.message?access_token=%s"
	// Posts as HTML, needs to know and use a body type
	BodyStart = "<body>"
	BodyEnd   = "</body>"
	Body      = BodyStart + "%s" + BodyEnd
	// Environment keys
	SynapseKey  = "SYNAPSE_"
	PhabUrlKey  = SynapseKey + "PHAB_URL"
	ApiTokenKey = SynapseKey + "API_TOKEN"
	FeedRoomKey = SynapseKey + "FEED_ROOM"
	HostKey     = SynapseKey + "HOST"
	DebugKey    = SynapseKey + "FEED_DEBUG"
	ConduitKey  = SynapseKey + "PHAB_TOKEN"
	ResolveKey  = SynapseKey + "FEED_PHIDS"
	LookupsKey  = SynapseKey + "LOOKUP_PHID"
	LogFileDir  = SynapseKey + "FEED_LOG"
	// PHID types
	IsPHIDType = "PHID-"
	// JSON keys
	ResultJSON = "result"
	ErrorJSON  = "error_code"
	// Feed keywords for news stories
	IsTag   = "tag"
	IsTitle = "title"
	// Build indicator
	Version = "1.2.0"
)

// Input configuration
type Config struct {
	phids     string
	room      string
	debug     bool
	conduit   string
	resolving []string
	lookups   map[string]string
	cache     *sync.Map
	paste     string
	logger    *Logging
	logDir    string
	url       string
	token     string
}

// Logging object
type Logging struct {
	sync.RWMutex
}

// Build a query string for key/value pair
func buildQuery(key string, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

// Write log data
func writeLog(category string, message string, conf *Config) {
	writeRawLog(category, message, conf, "")
}

// write an error out
func writeError(message string, err error, conf *Config) {
	if err != nil {
		goutils.WriteError(message, err)
	} else {
		goutils.WriteWarn(message)
	}
	go writeLogError(message, conf)
}

// write to file
func writeLogError(message string, conf *Config) {
	t := time.Now()
	category := t.Format("2006-01-02 15:04:05") + " [ERROR] "
	writeRawLog(category, message, conf, "error.")
}

// write raw logs
func writeRawLog(category string, message string, conf *Config, prefix string) {
	conf.logger.Lock()
	defer conf.logger.Unlock()
	t := time.Now()
	logFile := prefix + "phab-http." + t.Format("2006-01-02") + ".log"
	f, err := os.OpenFile(path.Join(conf.logDir, logFile), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		goutils.WriteError("unable to access log", err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s -> %s\n", category, message)
}

// Post with a form body
func postBody(data map[string]string, url string, conf *Config) []byte {
	var results []byte
	var datum []string
	datum = append(datum, buildQuery("api.token", conf.conduit))
	for k, v := range data {
		datum = append(datum, buildQuery(k, v))
	}
	var queryString = strings.Join(datum, "&")
	body := strings.NewReader(queryString)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		writeError("requesting", err, conf)
	} else {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			writeError("go", err, conf)
		} else {
			results, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				writeError("query", err, conf)
			} else {
				defer resp.Body.Close()
			}
		}
	}
	return results
}

// POST JSON data
func postJSON(data map[string]string, url string, conf *Config) {
	b, err := json.Marshal(data)
	if err != nil {
		writeError("json", err, conf)
		return
	}
	_, err = http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		writeError("req", err, conf)
	}
}

// Execute an actual posting to the synapse endpoint
func execute(text string, url string, conf *Config, phids []string) {
	m := make(map[string]string)
	m["msgtype"] = "m.text"
	m["body"] = BodyStart
	m["format"] = "org.matrix.custom.html"
	val := html.EscapeString(text)
	if len(phids) > 0 {
		val = val + "<br /> (references: " + strings.Join(phids, ", ") + ")"
	}
	m["formatted_body"] = fmt.Sprintf(Body, val)
	postJSON(m, url, conf)
}

// Check if a string represents a phid
func isPHID(value string, phids []string) bool {
	var val bool = false
	if strings.HasPrefix(value, IsPHIDType) {
		for _, element := range phids {
			if strings.HasPrefix(value, IsPHIDType+element+"-") {
				val = true
				break
			}
		}
	}
	return val
}

// support pulling raw JSON from an input byte stream
func getJSON(obj []byte, description string, errorKey bool, conf *Config) (bool, map[string]json.RawMessage) {
	var output map[string]json.RawMessage
	err := json.Unmarshal(obj, &output)
	var valid bool = true
	if err != nil {
		valid = false
		writeError(description, err, conf)
	} else {
		if conf.debug {
			if val, ok := output[ErrorJSON]; ok && val != nil && len(val) > 0 {
				writeError("error detected", nil, conf)
				writeError(string(val), nil, conf)
			}
		}
	}
	return valid, output
}

func digJSONOut(obj []byte, description string, conf *Config, dig []string) (bool, map[string]json.RawMessage) {
	valid, res := getJSON(obj, description, false, conf)
	if valid {
		if len(dig) > 0 {
			if conf.debug {
				goutils.WriteDebug("deeper", dig...)
			}
			var sub []string
			if len(dig) > 1 {
				for idx, element := range dig {
					if idx == 0 {
						continue
					}
					sub = append(sub, element)
				}
			}
			valid, res = digJSONOut(res[dig[0]], description, conf, sub)
		}
	}
	if !valid {
		goutils.WriteInfo("unable to dig out, see ^^^")
	}
	return valid, res
}

// init lookups
func initLookups(conf *Config, phid string) map[string]string {
	m := make(map[string]string)
	m["queryKey"] = "active"
	m["attachments[content]"] = "1"
	m["constraints[phids][0]"] = phid
	lookups := make(map[string]string)
	obj := postBody(m, conf.paste, conf)
	valid, output := getJSON(obj, "PHIDs", true, conf)
	if valid {
		valid, res := getJSON(output[ResultJSON], "Results", false, conf)
		if valid {
			var content []json.RawMessage
			err := json.Unmarshal(res["data"], &content)
			if err != nil {
				writeError("unable to read paste", err, conf)
			} else {
				if len(content) == 1 {
					valid, res = digJSONOut(content[0], "contents", conf, []string{"attachments", "content"})
					if valid {
						var data string
						err := json.Unmarshal(res["content"], &data)
						if err != nil {
							writeError("no final count found", err, conf)
						} else {
							err := json.Unmarshal([]byte(data), &lookups)
							if err != nil {
								writeError("invalid paste json", err, conf)
							} else {
								goutils.WriteDebug("lookups resolved")
							}
						}
					}
				} else {
					writeError("incorrect paste count", nil, conf)
				}
			}
		}
	}

	return lookups
}

func getMatrixPost(conf *Config, room string) string {
	return fmt.Sprintf(MatrixPost, conf.url, room, conf.token)
}

// Resolve phids
func resolvePHIDs(resolving []string, conf *Config) []string {
	var phids []string
	for _, element := range resolving {
		if conf.debug {
			goutils.WriteDebug(element)
		}
		if _, ok := conf.cache.Load(element); !ok {
			if conf.debug {
				goutils.WriteDebug("resolving...")
			}
			phids = append(phids, element)
		}
	}
	if len(phids) > 0 {
		if conf.debug {
			goutils.WriteDebug("calling to resolve phids")
		}
		m := make(map[string]string)
		var idx int = 0
		for _, element := range phids {
			m["phids["+strconv.Itoa(idx)+"]"] = element
			idx++
		}
		obj := postBody(m, conf.phids, conf)
		valid, output := getJSON(obj, "PHIDs", true, conf)
		if valid {
			valid, res := getJSON(output[ResultJSON], "Results", false, conf)
			if valid {
				for _, v := range res {
					var final map[string]string
					err := json.Unmarshal(v, &final)
					if err != nil {
						writeError("object", err, conf)
						continue
					}
					var name string = final["name"]
					var uri string = final["uri"]
					var resolved []string

					resolved = append(resolved, "<a href='"+uri+"'>"+html.EscapeString(name)+"</a>")
					if val, ok := conf.lookups[name]; ok {
						resolved = append(resolved, "aka: "+strings.Replace(val, ",", " ", -1))
					}
					conf.cache.Store(final["phid"], resolved)
				}
			}
		}
	}
	var results []string
	for _, element := range resolving {
		var writeRefs []string
		if inter, ok := conf.cache.Load(element); ok {
			val := inter.([]string)
			for _, item := range val {
				writeRefs = append(writeRefs, item)
				results = append(results, item)
			}
		}
		go writeLog(element, strings.Join(writeRefs, " "), conf)
	}

	if len(results) > 0 {
		sort.Strings(results)
	}

	return results
}

// Called when phabricator fires into the hook
func postStory(w http.ResponseWriter, r *http.Request, conf *Config) {
	r.ParseForm()
	var isStory bool = false
	var phids []string
	var story []string
	var toRoom string
	toRoom = conf.room
	var isTagged bool = false
	for k, v := range r.Form {
		if conf.debug {
			goutils.WriteDebug("kv: "+k, v...)
		}
		if len(v) > 0 {
			if k == "storyText" {
				isStory = true
				for _, element := range v {
					story = append(story, element)
				}
			} else {
				for _, element := range v {
					if isPHID(element, conf.resolving) {
						if conf.debug {
							goutils.WriteDebug("phid:", element)
						}
						phids = append(phids, element)
					} else {
						if k == "storyType" && element == "PhabricatorFeedTaggedStory" {
							isTagged = true
						}
					}
				}
			}
		}
	}
	if isStory {
		if len(phids) > 0 {
			if isTagged {
				phids = phids[:0]
			} else {
				if conf.debug {
					goutils.WriteDebug("resolving phids")
				}
				phids = resolvePHIDs(phids, conf)
			}
		}
		var addedStory string
		addedStory = ""
		storyText := strings.Join(story, "")
		if isTagged {
			if conf.debug {
				goutils.WriteDebug("story", storyText, toRoom)
			}
			var output map[string]string
			err := json.Unmarshal([]byte(storyText), &output)
			if err != nil {
				writeError("unable to read tagged story", err, conf)
				return
			}
			isValid := false
			if tagged, ok := output[IsTag]; ok {
				for k, v := range output {
					if k == IsTag {
						continue
					} else {
						if k == IsTitle {
							isValid = true
							toRoom = getMatrixPost(conf, tagged)
							storyText = v
						} else {
							addedStory = fmt.Sprintf("%s (%s -> %s)", addedStory, k, v)
						}
					}
				}

				storyText = fmt.Sprintf("%s%s", storyText, addedStory)
			}
			if !isValid {
				writeError("unable to parse tagged story", nil, conf)
				writeError(storyText, nil, conf)
				return
			}
		}
		if conf.debug {
			goutils.WriteDebug("routing", storyText, toRoom)
			if isTagged {
				goutils.WriteDebug("tagged")
			}
		}
		execute(storyText, toRoom, conf, phids)
	}
}

// main-entry point
func main() {
	vers := fmt.Sprintf("version: %s", Version)
	goutils.WriteInfo(fmt.Sprintf("Starting phab-http receiving hook (%s)", vers))
	conf := new(Config)
	url := os.Getenv(PhabUrlKey) + "api/"
	conf.phids = url + "phid.query"
	matrix := os.Getenv(HostKey)
	token := os.Getenv(ApiTokenKey)
	room := os.Getenv(FeedRoomKey)
	conf.paste = url + "paste.search"
	conf.conduit = os.Getenv(ConduitKey)
	conf.resolving = strings.Split(os.Getenv(ResolveKey), ",")
	conf.url = matrix
	conf.token = token
	conf.room = getMatrixPost(conf, room)
	conf.logDir = os.Getenv(LogFileDir)
	conf.logger = &Logging{}
	lookups := os.Getenv(LookupsKey)
	conf.cache = new(sync.Map)
	debug, err := strconv.ParseBool(os.Getenv(DebugKey))
	if err != nil {
		goutils.WriteError("unable to determine debug setting", err)
		conf.debug = false
	} else {
		conf.debug = debug
	}
	goutils.ConfigureLogging(conf.debug, true, true, true, false)
	if conf.debug {
		goutils.WriteDebug("debug on")
		goutils.WriteDebug("phids", conf.phids)
		goutils.WriteDebug("resolving", conf.resolving...)
		goutils.WriteDebug("api", conf.conduit, conf.url, conf.token, conf.room, conf.paste)
		goutils.WriteDebug("lookups")
		for k, v := range lookups {
			goutils.WriteDebug(strconv.Itoa(k), string(v))
		}
		goutils.WriteDebug("logging", conf.logDir)
		goutils.WriteDebug("init lookups...")
	}
	conf.lookups = initLookups(conf, lookups)
	if conf.debug {
		goutils.WriteDebug("lookups ready")
		for k, v := range conf.lookups {
			goutils.WriteDebug(k, string(v))
		}
	}
	writeLog("startup", "started", conf)
	http.HandleFunc("/alive", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, vers)
	})
	http.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		os.Exit(0)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		postStory(w, r, conf)
	})
	goutils.WriteInfo("started")
	listen := http.ListenAndServe(":8080", nil)
	if listen != nil {
		goutils.WriteError("listen failure", listen)
	}
}
