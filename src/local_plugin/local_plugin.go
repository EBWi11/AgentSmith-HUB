package local_plugin

import (
	"AgentSmith-HUB/local_plugin/cidr_match"
	"AgentSmith-HUB/local_plugin/is_private_ip"
	"AgentSmith-HUB/local_plugin/parse_json_data"

	// time plugins
	tday "AgentSmith-HUB/local_plugin/time/dayofweek"
	thour "AgentSmith-HUB/local_plugin/time/hourofday"
	tnow "AgentSmith-HUB/local_plugin/time/now"
	tdate "AgentSmith-HUB/local_plugin/time/timestamp_to_date"

	// encoding / hash
	b64dec "AgentSmith-HUB/local_plugin/encoding/base64_decode"
	b64enc "AgentSmith-HUB/local_plugin/encoding/base64_encode"
	hmd5 "AgentSmith-HUB/local_plugin/encoding/hash_md5"
	hsha1 "AgentSmith-HUB/local_plugin/encoding/hash_sha1"
	hsha256 "AgentSmith-HUB/local_plugin/encoding/hash_sha256"

	// url
	edomain "AgentSmith-HUB/local_plugin/url/extract_domain"
	esub "AgentSmith-HUB/local_plugin/url/extract_subdomain"
	etld "AgentSmith-HUB/local_plugin/url/extract_tld"

	// geo
	"AgentSmith-HUB/local_plugin/geo_match"

	// user agent
	pua "AgentSmith-HUB/local_plugin/user_agent/parse_user_agent"

	// string manipulation
	sreplace "AgentSmith-HUB/local_plugin/string/replace"

	// regex
	rextract "AgentSmith-HUB/local_plugin/regex/extract"
	rreplace "AgentSmith-HUB/local_plugin/regex/replace"

	// alert suppression
	suppressonce "AgentSmith-HUB/local_plugin/suppress_once"

	// threat intelligence
	shodan "AgentSmith-HUB/local_plugin/shodan"
	threatbook "AgentSmith-HUB/local_plugin/threatbook"
	virustotal "AgentSmith-HUB/local_plugin/virustotal"
)

// for checknode
var LocalPluginBoolRes = map[string]func(...interface{}) (bool, error){
	"isPrivateIP":  is_private_ip.Eval,
	"cidrMatch":    cidr_match.Eval,
	"geoMatch":     geo_match.Eval,
	"suppressOnce": suppressonce.Eval,
}

// for append or other usage
var LocalPluginInterfaceAndBoolRes = map[string]func(...interface{}) (interface{}, bool, error){
	"parseJSON": parse_json_data.Eval,

	// time helpers
	"now":       tnow.Eval,
	"dayOfWeek": tday.Eval,
	"hourOfDay": thour.Eval,
	"tsToDate":  tdate.Eval,

	// encoding / hash
	"base64Encode": b64enc.Eval,
	"base64Decode": b64dec.Eval,
	"hashMD5":      hmd5.Eval,
	"hashSHA1":     hsha1.Eval,
	"hashSHA256":   hsha256.Eval,

	// url parsing
	"extractDomain":    edomain.Eval,
	"extractTLD":       etld.Eval,
	"extractSubdomain": esub.Eval,

	// user agent
	"parseUA": pua.Eval,

	// string manipulation
	"replace": sreplace.Eval,

	// regex
	"regexExtract": rextract.Eval,
	"regexReplace": rreplace.Eval,

	// threat intelligence
	"virusTotal": virustotal.Eval,
	"shodan":     shodan.Eval,
	"threatBook": threatbook.Eval,
}

var LocalPluginDesc = map[string]string{
	// check node
	"isPrivateIP":  "Check node: true if IP is private RFC1918/loopback/link-local. Args: ip string.",
	"cidrMatch":    "Check node: true if IP within CIDR. Args: ip string, cidr string.",
	"geoMatch":     "Check node: true if IP country ISO matches expected. Args: ip, countryISO.",
	"suppressOnce": "Check node: alert suppression. Args: key(any), windowSec, ruleid(optional). Returns true only first time within window. Use ruleid to isolate different rules.",

	// time append
	"now":       "Append: current time. Args: optional format (unix|ms|rfc3339).",
	"dayOfWeek": "Append: day of week 0-6 (Sun=0). Args: optional timestamp.",
	"hourOfDay": "Append: hour of day 0-23. Args: optional timestamp.",
	"tsToDate":  "Append: convert Unix timestamp to RFC3339 string. Args: timestamp int64.",

	// encoding / hash append
	"base64Encode": "Append: base64 encode string. Args: plain string.",
	"base64Decode": "Append: base64 decode string. Args: encoded string.",
	"hashMD5":      "Append: MD5 hex of string. Args: string.",
	"hashSHA1":     "Append: SHA1 hex of string. Args: string.",
	"hashSHA256":   "Append: SHA256 hex of string. Args: string.",

	// url append
	"extractDomain":    "Append: extract domain from URL/host. Args: urlOrHost string.",
	"extractTLD":       "Append: extract TLD from domain. Args: domain string.",
	"extractSubdomain": "Append: extract subdomain from host. Args: host string.",

	// ua
	"parseUA": "Append: parse user agent to map. Args: ua string.",

	// misc
	"parseJSON": "Append: parse JSON string into map. Args: json string.",

	// string manipulation
	"replace": "Append: replace all occurrences of substring. Args: input, old, new.",

	// regex
	"regexExtract": "Append: extract text using regex. Returns match or capture groups. Args: input, pattern.",
	"regexReplace": "Append: replace text using regex. Supports $1, $2 references. Args: input, pattern, replacement.",

	// threat intelligence
	"virusTotal": "Append: query VirusTotal for file hash reputation. Returns detection info with caching. Args: hash string (MD5/SHA1/SHA256), apiKey string (optional - fallback to VIRUSTOTAL_API_KEY env var).",
	"shodan":     "Append: query Shodan for IP address infrastructure info. Returns host details with caching. Args: ip string (IPv4/IPv6), apiKey string (optional - fallback to SHODAN_API_KEY env var).",
	"threatBook": "Append: query ThreatBook (微步在线) for threat intelligence. Returns comprehensive threat info with caching. Args: queryValue string, queryType string (ip/domain/file/url), apiKey string (optional - fallback to THREATBOOK_API_KEY env var).",
}
