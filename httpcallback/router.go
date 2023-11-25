// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package httpcallback

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/logger_util"
	"github.com/sirupsen/logrus"
)

var HttpLog *logrus.Entry

func init() {
	HttpLog = logger.HttpLog
}

// Route is the information for every URI.
type Route struct {
	// Name is the name of this Route.
	Name string
	// Method is the string for the HTTP method. ex) GET, POST etc..
	Method string
	// Pattern is the pattern of the URI.
	Pattern string
	// HandlerFunc is the handler function of this route.
	HandlerFunc gin.HandlerFunc
}

// Routes is the list of the generated Route.
type Routes []Route

// NewRouter returns a new router.
func NewRouter() *gin.Engine {
	router := logger_util.NewGinWithLogrus(logger.GinLog)
	AddService(router)
	return router
}

func AddService(engine *gin.Engine) *gin.RouterGroup {
	group := engine.Group("/namf-callback/v1")

	for _, route := range routes {
		switch route.Method {
		case "GET":
			group.GET(route.Pattern, route.HandlerFunc)
		case "POST":
			group.POST(route.Pattern, route.HandlerFunc)
		case "PUT":
			group.PUT(route.Pattern, route.HandlerFunc)
		case "PATCH":
			group.PATCH(route.Pattern, route.HandlerFunc)
		case "DELETE":
			group.DELETE(route.Pattern, route.HandlerFunc)
		}
	}
	return group
}

// Index is the index handler.
func Index(c *gin.Context) {
	c.String(http.StatusOK, "Hello World!")
}

var routes = Routes{
	{
		"Index",
		"GET",
		"/",
		Index,
	},

	{
		"SmContextStatusNotify",
		strings.ToUpper("Post"),
		"/smContextStatus/:guti/:pduSessionId",
		HTTPSmContextStatusNotify,
	},

	{
		"AmPolicyControlUpdateNotifyUpdate",
		strings.ToUpper("Post"),
		"/am-policy/:polAssoId/update",
		HTTPAmPolicyControlUpdateNotifyUpdate,
	},

	{
		"AmPolicyControlUpdateNotifyTerminate",
		strings.ToUpper("Post"),
		"/am-policy/:polAssoId/terminate",
		HTTPAmPolicyControlUpdateNotifyTerminate,
	},

	{
		"N1MessageNotify",
		strings.ToUpper("Post"),
		"/n1-message-notify",
		HTTPN1MessageNotify,
	},
	{
		"NfStatusNotify",
		strings.ToUpper("Post"),
		"/nf-status-notify",
		HTTPNfSubscriptionStatusNotify,
	},
}
