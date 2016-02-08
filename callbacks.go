package grapi

import (
	"net/http"

	"github.com/jinzhu/gorm"
)

// Any model implementing the NeedsValidation interface will have this called
// on upload (ie. PATCH, and POST requests).
type NeedsValidation interface {
	ValidateUpload() map[string]string
}

// Common stuff for all Request handlers.
type RequestInfo interface {
	Param(string) string
	Method() string
	Options() *Options
	GetRequest() *http.Request
	GetData() interface{}
	SetData(d interface{})
}

// For callbacks that have the power to return an error.
type RequestResponseWriter interface {
	GetResponseWriter() http.ResponseWriter
}

type RequestLoginInfo interface {
	GetLoginObject() interface{}
}

// Authenticate - has all the info it wants apart from the LoginObject
// which doesn't exist yet. We need to set it.
type ReqToAuthenticate interface {
	RequestInfo
	RequestResponseWriter
	SetLoginObject(interface{})
}

// All info, and can decide whether to authorize so can write back.
type ReqToAuthorize interface {
	RequestInfo
	RequestLoginInfo
	RequestResponseWriter
}

// For limiting queries. Not allowed to write here.
type ReqToLimit interface {
	RequestInfo
	RequestLoginInfo
	GetDB() *gorm.DB
	SetDB(*gorm.DB)
}

// For checking the final result before it gets serialised
type ReqULToCheck interface {
	RequestInfo
	RequestLoginInfo
	RequestResponseWriter
	GetUpload() interface{}
}

type ReqFinalResult interface {
	RequestInfo
	RequestLoginInfo
	GetUpload() interface{}
	GetResult() interface{}
	SetResult(interface{})
}

// Authenticator is called as the first callback in a request. It has
// access to the http.Request as well as other info via the req object.
// An Authenticator should return true if the user has successfully
// authenticated themselves. Otherwise it should return false and
// use the http.ResponseWriter to explain why to the client.
type Authenticator func(req ReqToAuthenticate) bool

// Authorizor is a callback called after Authenticator, and should decide whether
// the authenticated user has permission to access the requested resource.
// If Authenticator has set the LoginObject with req.SetLoginObject() then
// this object is now available at req.GetLoginObject() to help decide.
// Typical usage might include:
//   user := req.GetLoginObject().(User) // User is the logged in user
//   if user.admin || user.id == req.Params("user_id")  {
//     return true
//   }
//   http.Error(req.GetResponseWriter(), "Only admin can access other users widgets", 403)
//   return false
type Authorizor func(req ReqToAuthorize) bool

// QueryLimiter is a callback to edit the gorm database used by the request using
// req.GetDB() and req.SetDB(). It can use this to scope the request eg:
//   req.SetDB( req.GetDB().Where("user_id = ?", req.Param("user_id")) )
// Return false to abandon request and optionally return custom data to client
type QueryLimiter func(req ReqToLimit) bool

// UploadChecker is a callback called for POST and PATCH requests. It has access
// to the parsed uploaded JSON via req.GetUpload(). If it returns false it will
// halt the request execution, and may return it's own data to the server.
type UploadChecker func(req ReqULToCheck) bool

// ResultEditor is the last callback called, immediately before data for the request is
// returned to the client. The data to be returned will be the result object serialised
// into json. This object can be retrieved by req.GetResult(), and edited or even completely
// replaced by req.SetResult(). If it returns false it will halt the request execution, and
// may return it's own data to the client.
type ResultEditor func(req ReqFinalResult) bool
