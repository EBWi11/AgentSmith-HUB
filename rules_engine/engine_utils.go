package rules_engine

import (
	"strconv"
	"strings"
)

func END(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return strings.HasSuffix(ori_data, check_data), check_data
}

func START(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return strings.HasPrefix(ori_data, check_data), check_data
}

func NEND(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return !strings.HasSuffix(ori_data, check_data), check_data
}

func NSTART(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return !strings.HasPrefix(ori_data, check_data), check_data
}

func INCL(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}
	return strings.Contains(ori_data, check_data), check_data
}

func NI(ori_data string, check_data string) (res bool, hitData string) {
	if ori_data == "" {
		return true, ""
	}
	return !strings.Contains(ori_data, check_data), check_data
}

func NCS_END(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return strings.HasSuffix(strings.ToLower(ori_data), strings.ToLower(check_data)), check_data
}

func NCS_START(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return strings.HasPrefix(strings.ToLower(ori_data), strings.ToLower(check_data)), check_data
}

func NCS_NEND(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return !strings.HasSuffix(strings.ToLower(ori_data), strings.ToLower(check_data)), check_data
}

func NCS_NSTART(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}

	return !strings.HasPrefix(strings.ToLower(ori_data), strings.ToLower(check_data)), check_data
}

func NCS_INCL(ori_data string, check_data string) (res bool, hitData string) {
	if check_data == "" {
		return true, check_data
	}

	if ori_data == "" {
		return false, ""
	}
	return strings.Contains(strings.ToLower(ori_data), strings.ToLower(check_data)), check_data
}

// NI Not Include，判断是否不包含特定字符
func NCS_NI(ori_data string, check_data string) (res bool, hitData string) {
	if ori_data == "" {
		return true, ""
	}
	return !strings.Contains(strings.ToLower(ori_data), strings.ToLower(check_data)), check_data
}

// MT More than，判断是否大于
func MT(ori_data string, check_data string) (res bool, hitData string) {
	ori_int, err := strconv.ParseFloat(ori_data, 64)
	if err != nil {
		return false, ""
	}
	check_int, err := strconv.ParseFloat(check_data, 64)
	if err != nil {
		return false, ""
	}

	if ori_int > check_int {
		return true, check_data
	} else {
		return false, ""
	}
}

// LT Less than，判断是否小于
func LT(ori_data string, check_data string) (res bool, hitData string) {
	ori_int, err := strconv.ParseFloat(ori_data, 64)
	if err != nil {
		return false, ""
	}
	check_int, err := strconv.ParseFloat(check_data, 64)
	if err != nil {
		return false, ""
	}

	if ori_int < check_int {
		return true, check_data
	} else {
		return false, ""
	}
}

func REGEX(ori_data string, regexCompile *regexp.Regex) (res bool, hitData string) {
	start, end, tmp_res := regexCompile.Find(ori_data)
	if tmp_res {
		return true, ori_data[start:end]
	} else {
		return false, ""
	}
}

func ISNULL(ori_data string) (res bool, hitData string) {
	if ori_data == "" {
		return true, ori_data
	} else {
		return false, ""
	}
}

func NOTNULL(ori_data string) (res bool, hitData string) {
	if strings.TrimSpace(ori_data) == "" {
		return false, ""
	} else {
		return true, ori_data
	}
}

func EQU(ori_data string, check_data string) (res bool, hitData string) {
	return strings.EqualFold(ori_data, check_data), ori_data
}

func NEQ(ori_data string, check_data string) (res bool, hitData string) {
	return !strings.EqualFold(ori_data, check_data), check_data
}

func NCS_EQU(ori_data string, check_data string) (res bool, hitData string) {
	return strings.EqualFold(strings.ToLower(ori_data), strings.ToLower(check_data)), ori_data
}

func NCS_NEQ(ori_data string, check_data string) (res bool, hitData string) {
	return !strings.EqualFold(strings.ToLower(ori_data), strings.ToLower(check_data)), check_data
}
