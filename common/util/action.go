package util

import (
	"strings"

	"github.com/ikenchina/octopus/common/http"
)

// split by ";"
// http://octopus.com or http://localhost
// grpc;xds://octopus.com;sagaservice.SagaService/Commit
func ExtractAction(action string) (string, string, string) {
	if strings.HasPrefix(action, "http") {
		idx := strings.Index(action, ":")
		if idx == -1 {
			return "", "", ""
		}
		scheme := action[:idx]
		return scheme, "", ""
	} else if strings.HasPrefix(action, "grpc") {
		spl := strings.Split(action, ";")
		if len(spl) != 3 {
			return "", "", ""
		}
		return spl[0], spl[1], spl[2]
	}
	return "", "", ""
}

func IsValidAction(action string) bool {
	scheme, domain, method := ExtractAction(action)
	if len(scheme) == 0 {
		return false
	}

	switch scheme {
	case "http", "https":
		return http.IsValidUrl(action)
	case "grpc":
		return domain != "" && method != ""
	}
	return true
}
