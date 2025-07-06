package geo_match

import (
	"errors"
	"net"
	"os"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

var (
	db    *geoip2.Reader
	dbErr error
)

func init() {
	dbPath := os.Getenv("GEOIP_DB")
	if dbPath == "" {
		dbPath = "/usr/share/GeoIP/GeoLite2-Country.mmdb"
	}
	db, dbErr = geoip2.Open(dbPath)
}

// Eval returns true if ipStr geo country ISO matches expectedCountry (case-insensitive).
// Args: ipStr, expectedCountryISO (e.g., "US").
func Eval(args ...interface{}) (bool, error) {
	if len(args) != 2 {
		return false, errors.New("geo_match requires 2 arguments: ip, countryISO")
	}
	ipStr, ok1 := args[0].(string)
	want, ok2 := args[1].(string)
	if !ok1 || !ok2 {
		return false, errors.New("arguments must be strings")
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false, nil
	}

	if dbErr != nil {
		return false, dbErr
	}
	if db == nil {
		return false, errors.New("geoip database not loaded")
	}
	rec, err := db.Country(ip)
	if err != nil {
		return false, err
	}
	if rec == nil || rec.Country.IsoCode == "" {
		return false, nil
	}
	return strings.EqualFold(rec.Country.IsoCode, want), nil
}
