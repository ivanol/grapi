/*
Package grapi implements a RESTful API web server for go.

Grapi is designed to be interoperable with the golang http.Handler
interface, and other software that works with this. The main features
are:

  * Works with gorm to access the backend databases, so will work with
    any of the backends supported by that (including sqlite, MySql and
    Postgresql)
  * Supports callbacks at each stage of the request process to allow for
    authentication, authorization, scoping, and editign queries
  * Allows for callbacks to be set on an application, resource, or individual
    endpoint level
*/
package grapi

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
	"github.com/zenazn/goji/web/middleware"

	log "github.com/Sirupsen/logrus"
)

// Options which apply to all routes served by a Grapi instance
type Options struct {
	// DB is the gorm database that will be used to access resources. If this is nil then there will be a panic
	// on startup.
	Db *gorm.DB

	// JwtKey is required if you are using the inbuilt authentication system (ie. passing UseDefaultAuth: true to
	// any of your routes. It should be a cryptographically secure random string that is not checked into source
	// code, but retrieved securely on startup from a config file or environment variable.
	JwtKey string

	// If you are going to UseDefaultAuth then you also need to provide an object which satisfies LoginModel. This
	// provides us with callbacks that will be used to check login credentials, and to retrieve the User model
	// for passing to subsequent callbacks.
	LoginModel LoginModel

	// UriPrefix is optional and defaults to api. The REST routes for a model called ModelName will be found by
	// default at /UriModelName/model_names . A single leading or trailing slash on UriPrefix will be ignored.
	// Note that you will have to also tell your router to route http requests for routes starting with UriPrefix
	// to Grapi
	UriPrefix string

	// For debugging. Adds this number of milliseconds latency to every api request so you can check your
	// app remains responsive. Doesn't work at present
	httpLatency int

	// LogLevel. 0 is default (errors). -1 means silent. 1 means everything.
	LogLevel int
}

// Grapi is an http handler which handles REST requests for objects it has been
// asked to. Retrieve a Grapi object by passing an Options structure to New
type Grapi struct {
	router  *web.Mux
	db      *gorm.DB
	options *Options
	prefix  string
}

// New returns a new Grapi object intialised with options. Options must contain
// a value for Db, and if the inbuilt authentication is being used should contain
// a value for JwtKey and LoginModel.
func New(o Options) *Grapi {
	if o.Db == nil {
		panic("Must provide a non nil Db object in the options for a new Grapi")
	}
	if o.UriPrefix == "" {
		o.UriPrefix = "/api"
	}
	if o.UriPrefix[0] != '/' {
		o.UriPrefix = "/" + o.UriPrefix
	}
	o.UriPrefix = strings.TrimSuffix(o.UriPrefix, "/")

	gj := web.New()
	gj.Use(middleware.RequestID)
	if o.LogLevel > 0 {
		gj.Use(middleware.Logger)
	}
	gj.Use(middleware.Recoverer)
	gj.Use(middleware.AutomaticOptions)

	goji.Handle(o.UriPrefix, gj)
	api := Grapi{router: gj, options: &o, db: o.Db}
	return &api
}

// DB returns the underlying DB object
func (g *Grapi) DB() *gorm.DB {
	return g.db
}

// We are an http handler
func (g *Grapi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.router.ServeHTTP(w, r)
}

// Add REST routes for model. If Grapi was initialised with the default UriPrefix="api"
// and AddDefaultRoutes is called with a model called SecretWidget like this:
//   g.AddDefaultRoutes(&SecretWidget{})
// then we will add the following routes:
//   * GET /api/secret_widgets  - Return a list of all SecretWidget objects
//   * GET /api/secret_widgets/:id  - Return SecretWidget with ID==:id, or 404 Not Found
//   * POST /api/secret_widgets  - Upload a new SecretWidget to the database, or return 422 if posted json doesn't parse
//   * PATCH /api/secret_widgets/:id  - Update SecretWidget with ID==:id, or return 422 or 404 on error
//   * DELETE /api/secret_widgets/:id  - Delete the SecretWidget with ID==:id
//
// options is optional. If two options arguments
// are given then the first will apply to GET routes, and the second to POST/PATCH/DELETE
// If three are given then the third applies to DELETE routes
func (g *Grapi) AddDefaultRoutes(modelPtr interface{}, options ...RouteOptions) {
	var viewOption *RouteOptions
	if len(options) > 3 {
		panic("AddDefaultRoutes called with more than 3 RouteOptions")
	}
	if len(options) > 0 {
		viewOption = &options[0]
	}
	editOption := viewOption
	if len(options) > 1 {
		editOption = &options[1]
	}
	deleteOption := editOption
	if len(options) > 2 {
		deleteOption = &options[2]
	}
	g.AddGetRoute(modelPtr, viewOption)
	g.AddIndexRoute(modelPtr, viewOption)
	g.AddPostRoute(modelPtr, editOption)
	g.AddPatchRoute(modelPtr, editOption)
	g.AddDeleteRoute(modelPtr, deleteOption)
}

// Adds a route at "#{g.prefix}/#{pluralmodelname}/:id" to get item. ie. g.AddGetRoute(&Widget{}, nil)
// by default adds a route such that GET "/api/widgets/2" returns the widget with id==2
func (g *Grapi) AddGetRoute(modelP interface{}, ro *RouteOptions) {
	if ro == nil {
		ro = &RouteOptions{}
	}
	ro.Initialise(g)
	path := g.makePath(modelP, ro) + "/:id"
	modelType := reflect.TypeOf(modelP).Elem()
	log.WithFields(log.Fields{"Model": modelType, "path": path}).Info("Adding GET route")
	g.router.Get(path, g.itemHandler(modelType, ro))
}

// Adds a route at "#{g.prefix}/#{pluralmodelname}/:id" to get item. ie. g.AddIndexRoute(&Widget{}, nil)
// by default adds a route such that GET "/api/widgets" returns a list of all widgets
func (g *Grapi) AddIndexRoute(modelP interface{}, ro *RouteOptions) {
	if ro == nil {
		ro = &RouteOptions{}
	}
	ro.Initialise(g)
	path := g.makePath(modelP, ro)
	modelType := reflect.TypeOf(modelP).Elem()
	sliceType := reflect.SliceOf(modelType)
	log.WithFields(log.Fields{"Model": modelType, "path": path}).Info("Adding INDEX route")
	g.router.Get(path, g.indexHandler(sliceType, ro))
}

// Adds a route at "#{g.prefix}/#{pluralmodelname}" to insert an item. ie. g.AddGetRoute(&Widget{}, nil)
// by default adds a route such that POST "/api/widgets/2" inserts a widget to the database
func (g *Grapi) AddPostRoute(modelP interface{}, ro *RouteOptions) {
	if ro == nil {
		ro = &RouteOptions{}
	}
	ro.Initialise(g)
	path := g.makePath(modelP, ro)
	modelType := reflect.TypeOf(modelP).Elem()
	log.WithFields(log.Fields{"Model": modelType, "path": path}).Info("Adding POST route")
	g.router.Post(path, g.postHandler(modelType, ro))
}

// Adds a route at "#{g.prefix}/#{pluralmodelname}/:id" to edit an item. ie. g.AddGetRoute(&Widget{}, nil)
// by default adds a route such that PATCH "/api/widgets/2" edits the widget with id==2
func (g *Grapi) AddPatchRoute(modelP interface{}, ro *RouteOptions) {
	if ro == nil {
		ro = &RouteOptions{}
	}
	ro.Initialise(g)
	path := g.makePath(modelP, ro) + "/:id"
	modelType := reflect.TypeOf(modelP).Elem()
	log.WithFields(log.Fields{"Model": modelType, "path": path}).Info("Adding PATCH route")
	g.router.Patch(path, g.patchHandler(modelType, ro))
}

// Adds a route at "#{g.prefix}/#{pluralmodelname}/:id" to delete item. ie. g.AddGetRoute(&Widget{}, nil)
// by default adds a route such that DELETE "/api/widgets/2" deletes the widget with id==2
func (g *Grapi) AddDeleteRoute(modelP interface{}, ro *RouteOptions) {
	if ro == nil {
		ro = &RouteOptions{}
	}
	ro.Initialise(g)
	path := g.makePath(modelP, ro) + "/:id"
	modelType := reflect.TypeOf(modelP).Elem()
	log.WithFields(log.Fields{"Model": modelType, "path": path}).Info("Adding DELETE route")
	g.router.Delete(path, g.deleteHandler(modelType, ro))
}

// itemHandler returns a goji handler that gets a single item from the database and returns it.
// Depending on the callbacks set in RouteOptions, before querying the database we may authenticate,
// authorize, and scope the request. After query the result may be edited before sending back to
// client. 404/422/500 etc are sent as appropriate on error.
func (g *Grapi) itemHandler(itemType reflect.Type, o *RouteOptions) web.HandlerType {
	tableName := pluralCamelNameType(itemType)
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		req := request{api: g, DB: g.db, method: "GET", C: c, W: w, R: r, Type: itemType, TableName: tableName, options: o}
		if true &&
			(o.Authenticate == nil || o.Authenticate(&req)) &&
			(o.Authorize == nil || o.Authorize(&req)) &&
			(o.Query == nil || o.Query(&req)) &&
			req.GetItemById() &&
			(o.EditResult == nil || o.EditResult(&req)) &&
			req.SerialiseResult() {
			log.WithFields(log.Fields{"Model": itemType}).Info("Successful GET")
		}
	}
}

// indexHandler does the same as itemHandler, but for a full list of items.
func (g *Grapi) indexHandler(sliceType reflect.Type, o *RouteOptions) web.HandlerType {
	tableName := pluralCamelNameType(sliceType)
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		req := request{api: g, DB: g.db, method: "GET", C: c, W: w, R: r, Type: sliceType, TableName: tableName, options: o}
		if true &&
			(o.Authenticate == nil || o.Authenticate(&req)) &&
			(o.Authorize == nil || o.Authorize(&req)) &&
			(o.Query == nil || o.Query(&req)) &&
			req.GetItems() &&
			(o.EditResult == nil || o.EditResult(&req)) &&
			req.SerialiseResult() {
			log.WithFields(log.Fields{"Model": sliceType}).Info("Successful index GET")
		}
	}
}

// postHandler returns a handler for posting a new item to the database.
func (g *Grapi) postHandler(itemType reflect.Type, o *RouteOptions) web.HandlerType {
	tableName := pluralCamelNameType(itemType)
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		req := request{api: g, DB: g.db, method: "POST", C: c, W: w, R: r, Type: itemType, TableName: tableName, options: o}
		if true &&
			(o.Authenticate == nil || o.Authenticate(&req)) &&
			(o.Authorize == nil || o.Authorize(&req)) &&
			req.ParseUpload() &&
			(o.CheckUpload == nil || o.CheckUpload(&req)) &&
			req.PostDB() &&
			(o.EditResult == nil || o.EditResult(&req)) &&
			req.SerialiseResult() {
			log.WithFields(log.Fields{"Model": itemType}).Info("Successful POST")
		}
	}
}

// patchHandler returns a handler for editing items in the database
func (g *Grapi) patchHandler(itemType reflect.Type, o *RouteOptions) web.HandlerType {
	tableName := pluralCamelNameType(itemType)
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		req := request{api: g, DB: g.db, method: "PATCH", C: c, W: w, R: r, Type: itemType, TableName: tableName, options: o}
		if true &&
			(o.Authenticate == nil || o.Authenticate(&req)) &&
			(o.Authorize == nil || o.Authorize(&req)) &&
			(o.Query == nil || o.Query(&req)) &&
			req.GetItemById() &&
			req.PatchResultWithUploaded() &&
			(o.CheckUpload == nil || o.CheckUpload(&req)) &&
			req.PatchDB() &&
			(o.EditResult == nil || o.EditResult(&req)) &&
			req.SerialiseResult() {
			log.WithFields(log.Fields{"Model": itemType}).Info("Successful PATCH")
		}
	}
}

// deleteHandler returns a handler for deleting items from the database.
func (g *Grapi) deleteHandler(itemType reflect.Type, o *RouteOptions) web.HandlerType {
	tableName := pluralCamelNameType(itemType)
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		req := request{api: g, DB: g.db, method: "DELETE", C: c, W: w, R: r, Type: itemType, TableName: tableName, options: o}
		if true &&
			(o.Authenticate == nil || o.Authenticate(&req)) &&
			(o.Authorize == nil || o.Authorize(&req)) &&
			(o.Query == nil || o.Query(&req)) &&
			req.GetItemById() &&
			req.DeleteFromDB() &&
			(o.EditResult == nil || o.EditResult(&req)) &&
			req.SerialiseResult() {
			log.WithFields(log.Fields{"Model": itemType}).Info("Successful DELETE")
		}
	}
}

// makePath returns the path for the item, modidified as required by any options.
func (g *Grapi) makePath(modelP interface{}, ro *RouteOptions) string {
	path := ro.UriModelName
	if len(path) == 0 {
		path = pluralCamelName(modelP)
	}
	return g.options.UriPrefix + ro.Prefix + "/" + path
}
