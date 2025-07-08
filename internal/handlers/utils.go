package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// stripJSONExtension removes .json extension from a parameter if present
func stripJSONExtension(c *gin.Context, paramName string) {
	value := c.Param(paramName)
	if strings.HasSuffix(value, ".json") {
		// Find the parameter index
		for i, param := range c.Params {
			if param.Key == paramName {
				c.Params[i].Value = strings.TrimSuffix(value, ".json")
				break
			}
		}
	}
}