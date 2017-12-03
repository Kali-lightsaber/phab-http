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
	"html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Posting messages to rooms
const MatrixPost = "%s/_matrix/client/r0/rooms/%s/send/m.room.message?access_token=%s"

// Posts as HTML, needs to know and use a body type
const BodyStart = "<body>"
const BodyEnd = "</body>"
const Body = BodyStart + "%s" + BodyEnd

// Provides conversion of T[0-9]+ naming to actual URLs

// Environment keys
const SynapseKey = "SYNAPSE_"
const PhabUrlKey = SynapseKey + "PHAB_URL"
const ApiTokenKey = SynapseKey + "API_TOKEN"
const FeedRoomKey = SynapseKey + "FEED_ROOM"
const HostKey = SynapseKey + "HOST"
const DebugKey = SynapseKey + "FEED_DEBUG"
const ConduitKey = SynapseKey + "PHAB_TOKEN"
const ResolveKey = SynapseKey + "FEED_PHIDS"
const LookupsKey = SynapseKey + "LOOKUP_PHID"
const LogFileDir = SynapseKey + "FEED_LOG"

// PHID types
const IsPHIDType = "PHID-"

// JSON keys
const ResultJSON = "result"
const ErrorJSON = "error_code"

// Feed keywords for news stories
const IsTag = "tag"
const IsTitle = "title"

// Build indicator
var build string

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
func writeLog(category string, message string, logging *Logging, dir string) {
	logging.Lock()
	defer logging.Unlock()
	t := time.Now()
	logFile := "phab-http." + t.Format("2006-01-02") + ".log"
	f, err := os.OpenFile(path.Join(dir, logFile), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Print("unable to access log", err)
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
		log.Print("Requesting: ", err)
	} else {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Print("Go: ", err)
		} else {
			results, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Print("QUERY: ", err)
			} else {
				defer resp.Body.Close()
				if conf.debug {
					log.Print(resp)
				}
			}
		}
	}
	return results
}

// POST JSON data
func postJSON(data map[string]string, url string, debug bool) {
	b, err := json.Marshal(data)
	if err != nil {
		log.Print("JSON: ", err)
	} else {
		resp, err := http.Post(url, "application/json", bytes.NewReader(b))
		if err != nil {
			log.Print("Req: ", err)
		} else {
			defer resp.Body.Close()
			if debug {
				log.Print(resp)
			}
		}
	}
}

// Execute an actual posting to the synapse endpoint
func execute(text string, url string, debug bool, phids []string) {
	m := make(map[string]string)
	m["msgtype"] = "m.text"
	m["body"] = BodyStart
	m["format"] = "org.matrix.custom.html"
	val := html.EscapeString(text)
	if len(phids) > 0 {
		val = val + "<br /> (references: " + strings.Join(phids, ", ") + ")"
	}
	m["formatted_body"] = fmt.Sprintf(Body, val)
	postJSON(m, url, debug)
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
func getJSON(obj []byte, description string, errorKey bool, debug bool) (bool, map[string]json.RawMessage) {
	var output map[string]json.RawMessage
	err := json.Unmarshal(obj, &output)
	var valid bool = true
	if err != nil {
		valid = false
		log.Print(description+": ", err)
	} else {
		if debug {
			if val, ok := output[ErrorJSON]; ok && val != nil && len(val) > 0 {
				log.Print("Error key detected")
				log.Print(string(val))
			}
		}
	}
	return valid, output
}

func digJSONOut(obj []byte, description string, debug bool, dig []string) (bool, map[string]json.RawMessage) {
	valid, res := getJSON(obj, description, false, debug)
	if valid {
		if len(dig) > 0 {
			if debug {
				log.Print("going deeper")
				log.Print(dig)
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
			valid, res = digJSONOut(res[dig[0]], description, debug, sub)
		}
	}
	if !valid {
		log.Print("Unable to dig out see ^^^")
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
	valid, output := getJSON(obj, "PHIDs", true, conf.debug)
	if valid {
		valid, res := getJSON(output[ResultJSON], "Results", false, conf.debug)
		if valid {
			var content []json.RawMessage
			err := json.Unmarshal(res["data"], &content)
			if err != nil {
				log.Print("Unable to read paste", err)
			} else {
				if len(content) == 1 {
					valid, res = digJSONOut(content[0], "contents", conf.debug, []string{"attachments", "content"})
					if valid {
						var data string
						err := json.Unmarshal(res["content"], &data)
						if err != nil {
							log.Print("no final content found")
						} else {
							err := json.Unmarshal([]byte(data), &lookups)
							if err != nil {
								log.Print("invalid paste json")
							} else {
								if conf.debug {
									log.Print("lookups resolved")
								}
							}
						}
					}
				} else {
					log.Print("Error - incorrect amount of pastes...")
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
			log.Print(element)
		}
		if _, ok := conf.cache.Load(element); !ok {
			if conf.debug {
				log.Print("calling to resolve")
			}
			phids = append(phids, element)
		}
	}
	if len(phids) > 0 {
		if conf.debug {
			log.Print("calling to resolve phids")
		}
		m := make(map[string]string)
		var idx int = 0
		for _, element := range phids {
			m["phids["+strconv.Itoa(idx)+"]"] = element
			idx++
		}
		obj := postBody(m, conf.phids, conf)
		valid, output := getJSON(obj, "PHIDs", true, conf.debug)
		if valid {
			valid, res := getJSON(output[ResultJSON], "Results", false, conf.debug)
			if valid {
				for _, v := range res {
					var final map[string]string
					err := json.Unmarshal(v, &final)
					if err != nil {
						log.Print("Object: ", err)
					} else {
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
		go writeLog(element, strings.Join(writeRefs, " "), conf.logger, conf.logDir)
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
			log.Print("===")
			log.Print(k)
			log.Print(v)
			log.Print("===")
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
							log.Print("is phid")
							log.Print(element)
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
					log.Print("Resolving phids")
				}
				phids = resolvePHIDs(phids, conf)
			}
		}
		var addedStory string
		addedStory = ""
		storyText := strings.Join(story, "")
		if isTagged {
			if conf.debug {
				log.Print(storyText)
				log.Print(toRoom)
			}
			var output map[string]string
			err := json.Unmarshal([]byte(storyText), &output)
			if err != nil {
				log.Print("unable to read tagged story", err)
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
				log.Print("unable to parse tagged story")
				log.Print(storyText)
			}
		}
		if conf.debug {
			log.Print(storyText)
			log.Print(isTagged)
			log.Print(toRoom)
		}
		execute(storyText, toRoom, conf.debug, phids)
	}
}

// main-entry point
func main() {
	vers := fmt.Sprintf("version: %s", build)
	log.Print(fmt.Sprintf("Starting phab-http receiving hook (%s)", vers))
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
		log.Print("Unable to determine debug setting: ", err)
		conf.debug = false
	} else {
		conf.debug = debug
	}
	if conf.debug {
		log.Print("debugging enabled")
		log.Print(conf.phids)
		log.Print(conf.resolving)
		log.Print(conf.conduit)
		log.Print(conf.url)
		log.Print(conf.token)
		log.Print(conf.room)
		log.Print(conf.paste)
		log.Print(lookups)
		log.Print(conf.logDir)
		log.Print("initialize lookups")
	}

	conf.lookups = initLookups(conf, lookups)
	if conf.debug {
		log.Print("lookups ready")
		log.Print(conf.lookups)
	}
	writeLog("startup", "started", conf.logger, conf.logDir)
	http.HandleFunc("/alive", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, vers)
	})
	http.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		os.Exit(0)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		postStory(w, r, conf)
	})
	log.Print("ready...")
	listen := http.ListenAndServe(":8080", nil)
	if listen != nil {
		log.Fatal("ListenAndServe: ", listen)
	}
}
