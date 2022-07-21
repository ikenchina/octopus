package http

import (
	"encoding/json"
	"net/url"

	"github.com/gin-gonic/gin"
)

func IsValidUrl(hurl string) bool {
	_, err := url.Parse(hurl)
	return err == nil
}

func ParseHttpJsonRequest(c *gin.Context, request interface{}) error {
	if request != nil {
		body, err := c.GetRawData()
		if err != nil {
			return err
		}
		err = json.Unmarshal(body, request)
		if err != nil {
			return err
		}
	}
	return nil
}
