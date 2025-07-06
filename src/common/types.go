package common

// CheckCoreCache for rule engine
type CheckCoreCache struct {
	Exist bool
	Data  string
}

type HubConfig struct {
	Redis         string `yaml:"redis"`
	RedisPassword string `yaml:"redis_password,omitempty"`
	ConfigRoot    string
	Leader        string
	LocalIP       string
	Token         string
}
