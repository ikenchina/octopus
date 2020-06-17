package http

import "net/url"

func IsValidUrl(hurl string) bool {
	_, err := url.Parse(hurl)
	return err == nil
}
