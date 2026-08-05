package controller

import "github.com/gin-gonic/gin"

func strictQueryFields(c *gin.Context, allowed map[string]bool, scalar ...string) map[string]string {
	values := c.Request.URL.Query()
	for key := range values {
		if !allowed[key] {
			return map[string]string{key: "unsupported"}
		}
	}
	for _, key := range scalar {
		if len(values[key]) > 1 {
			return map[string]string{key: "must be provided at most once"}
		}
	}
	return nil
}
