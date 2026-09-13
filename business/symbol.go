package business

import "fmt"

func toSymbol(code string) (string, error) {
	if len(code) != 6 {
		return "", fmt.Errorf("invalid stock code: %s", code)
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return "", fmt.Errorf("invalid stock code: %s", code)
		}
	}
	if code[0] == '6' {
		return "sh" + code, nil
	}
	if code[0] == '0' || code[0] == '3' {
		return "sz" + code, nil
	}
	return "", fmt.Errorf("unsupported stock code: %s", code)
}
