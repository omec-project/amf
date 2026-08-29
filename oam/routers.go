// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package oam

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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

func AddService(engine *gin.Engine) *gin.RouterGroup {
	group := engine.Group("/namf-oam/v1")

	for _, route := range routes {
		switch route.Method {
		case http.MethodGet:
			group.GET(route.Pattern, route.HandlerFunc)
		case http.MethodDelete:
			group.DELETE(route.Pattern, route.HandlerFunc)
		case http.MethodPost:
			group.POST(route.Pattern, route.HandlerFunc)
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
		http.MethodGet,
		"/",
		Index,
	},
	{
		"Registered UE Context",
		http.MethodGet,
		"/registered-ue-context",
		HTTPRegisteredUEContext,
	},

	{
		"Individual Registered UE Context",
		http.MethodGet,
		"/registered-ue-context/:supi",
		HTTPRegisteredUEContext,
	},
	{
		"Purge UE Context",
		http.MethodDelete,
		"/purge-ue-context/:supi",
		HTTPPurgeUEContext,
	},
	{
		"Active UE List",
		http.MethodGet,
		"/active-ues",
		HTTPGetActiveUes,
	},
	{
		"Amf Instance Down Notification",
		http.MethodPost,
		"/amfInstanceDown/:nfid",
		HTTPAmfInstanceDown,
	},
}
