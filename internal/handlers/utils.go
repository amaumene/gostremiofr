package handlers

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
)

// stripJSONExtension removes .json extension from a parameter if present
func stripJSONExtension(c *gin.Context, paramName string) {
	value := c.Param(paramName)
	if strings.HasSuffix(value, ".json") {
		for i, param := range c.Params {
			if param.Key == paramName {
				c.Params[i].Value = strings.TrimSuffix(value, ".json")
				break
			}
		}
	}
}

func decodeUserConfig(encodedConfig string) map[string]interface{} {
	var userConfig map[string]interface{}
	if data, err := base64.StdEncoding.DecodeString(encodedConfig); err == nil {
		json.Unmarshal(data, &userConfig)
	}
	return userConfig
}
