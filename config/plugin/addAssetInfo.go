package plugin

func Eval(ip string) (interface{}, bool, error) {
	res := make(map[string]interface{})

	if ip == "" {
		return res, false, nil
	} else if ip == "8.8.8.8" {
		res["type"] = "dns server"
	} else {
		res["type"] = "unknown"
	}

	return res, true, nil
}
