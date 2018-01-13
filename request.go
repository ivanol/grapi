package grapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/jinzhu/gorm"
	"github.com/zenazn/goji/web"

	log "github.com/Sirupsen/logrus"
)

// request is a per request structure, containing a db
// handle and a link to the API. The db handle may be altered by other
// handlers - eg. to add a Where or Limit clause.
// Once there is a result it will be stored in Result. This may be
// checked and/or edited by callbacks, and will then be returned to the
// caller by json.marshal(Result)
//
// For requests where a structure is being uploaded (POST/PUT/PATCH) this
// will be parsed and attached to 'Uploaded' after authentication.
type request struct {
	C         web.C
	W         http.ResponseWriter
	R         *http.Request
	Type      reflect.Type
	TableName string
	options   *RouteOptions

	DB          *gorm.DB
	api         *Grapi
	method      string // 'GET', 'POST', 'PUT', 'PATCH' or 'DELETE'
	Result      interface{}
	Uploaded    interface{}
	LoginObject interface{} // An object that describes the authenticated user.

	Data interface{} // User defined data that can be stored in the request object.
}

// First we have a whole series of functions that are useful to the callbacks, and which
// are required in order to implement the interfaces that the callback functions are
// expecting

//GetRequest returns the http.Request, fulfilling RequestInfo
func (r *request) GetRequest() *http.Request {
	return r.R
}

//Options returns the Options structure, fulfilling RequestInfo
func (r *request) Options() *Options {
	return r.api.options
}

// Param returns the param K from the URL, fulfilling RequestInfo
func (r *request) Param(k string) string {
	return r.C.URLParams[k]
}

// SetParam allows changing a URL Param. Only used in LimitQuery
func (r *request) SetParam(k string, v string) {
	r.C.URLParams[k] = v
}

func (r *request) Method() string {
	return r.method
}

// GetData gets the user defined data field from the request. This can contain anything
func (r *request) GetData() interface{} {
	return r.Data
}

// SetData gets the user defined data field from the request. This can contain anything
func (r *request) SetData(d interface{}) {
	r.Data = d
}

//GetResponseWriter returns the http.ResponseWriter fulfilling RequestResponseWriter
func (r *request) GetResponseWriter() http.ResponseWriter {
	return r.W
}

//GetLoginObject returns the object set with SetLoginObject fulfilling RequestLoginInfo.
//This will usually be a structure containing the details of the authenticated user.
func (r *request) GetLoginObject() interface{} {
	return r.LoginObject
}

//SetLoginObject is required by ReqToAuthenticate, and can be any object. It's only use
//internally is to be returned by GetLoginObject
func (r *request) SetLoginObject(lo interface{}) {
	r.LoginObject = lo
}

//GetDB() returns the underlying DB for this request, and is needed to fulfill ReqToLimit
func (r *request) GetDB() *gorm.DB {
	return r.DB
}

//SetDB() sets the underlying DB for this request and is needed to fulfill ReqToLimit
func (r *request) SetDB(db *gorm.DB) {
	r.DB = db
}

//GetUpload returns the uploaded json body deserialised into a pointer to
// the object that this route is built for. Fulfills ReqULToCheck
func (r *request) GetUpload() interface{} {
	return r.Uploaded
}

//GetResult() retrieves the result that will be serialised as the http response. Fulfills ReqFinalResult
func (r *request) GetResult() interface{} {
	return r.Result
}

//SetResult() sets the response. Needed for ReqFinalResult. Any object that can be serialised to JSON may
//be set as the result.
func (r *request) SetResult(result interface{}) {
	r.Result = result
}

// GetItemById gets the url param :id, retrieves the equivalent item from the database, and stores it in r.Result
func (r *request) GetItemById() bool {
	id := r.C.URLParams["id"]
	item := reflect.New(r.Type).Interface()
	qstring := fmt.Sprintf("%s.id = ?", r.TableName)
	if r.DB.Where(qstring, id).Find(item).RecordNotFound() {
		http.Error(r.W, "Not Found", 404)
		return false
	}
	r.Result = item
	return true
}

// GetItems retrieves all objects from db, and stores them as a slice in r.Result
func (r *request) GetItems() bool {
	items := getReflectedSlicePtr(r.Type)
	r.DB.Find(items)
	r.Result = items
	return true
}

// ParseUpload unserialises the uploaded html body (should be json) into an object
// of the type this route was built with.
func (r *request) ParseUpload() bool {
	body := httpBody(r.R)
	item := reflect.New(r.Type).Interface()
	if err := json.Unmarshal(body, item); err != nil {
		log.WithFields(log.Fields{"error": err}).Warn("Can't parse incoming json")
		r.W.WriteHeader(422) // unprocessable entity
		return false
	}
	r.Uploaded = item
	switch r.Uploaded.(type) {
	case NeedsValidation:
		err := r.Uploaded.(NeedsValidation).ValidateUpload()
		if err != nil && len(err) != 0 {
			log.WithFields(log.Fields{"error": err}).Warn("Validation error")
			j, _ := json.Marshal(err)
			http.Error(r.W, fmt.Sprintf(`{"errors":%v}`, string(j)), 422)
			return false
		}
	}
	return true
}

// PostDB saves the object in r.Uploaded to the db. We use the original DB object from
// API instead of r.DB as r.DB may have been edited with joins etc. and this breaks things.
func (r *request) PostDB() bool {
	uploaded := r.Uploaded
	log.Printf("upload is a %T\n", uploaded)

	post := r.api.db.Create(r.Uploaded)
	err := post.Error
	if err != nil {
		log.Warn("Error creating in doCreate: ", err)
		r.W.WriteHeader(422)
		return false
	}
	r.Result = r.Uploaded
	return true
}

// PatchResultWithUploaded unmarshals the uploaded item into r.Uploaded, merging it with
// the object in r.Result. GetItemById must have been called before this to fill r.Result
// with the contents of the existing item from the database.
func (r *request) PatchResultWithUploaded() bool {
	body := httpBody(r.R)
	beforeID, _ := getID(r.Result)
	if err := json.Unmarshal(body, r.Result); err != nil {
		log.WithFields(log.Fields{"error": err}).Warn("Can't parse incoming json")
		r.W.WriteHeader(422) // unprocessable entity
		return false
	}
	afterID, _ := getID(r.Result)
	if beforeID != afterID {
		log.WithFields(log.Fields{"afterID": afterID, "beforeID": beforeID}).Warn("Patch trying to change ID")
		r.W.WriteHeader(422) // unprocessable entity
		return false
	}
	r.Uploaded = r.Result
	switch r.Uploaded.(type) {
	case NeedsValidation:
		err := r.Uploaded.(NeedsValidation).ValidateUpload()
		if err != nil && len(err) != 0 {
			log.WithFields(log.Fields{"error": err}).Warn("Validation error")
			j, _ := json.Marshal(err)
			http.Error(r.W, fmt.Sprintf(`{"errors":%v}`, string(j)), 422)
			return false
		}
	}
	return true
}

// PatchDB saves the object in r.Uploaded to the db. We use the original DB object from
// API instead of r.DB as r.DB may have been edited with joins etc. and this breaks things.
func (r *request) PatchDB() bool {
	r.api.db.Save(r.Uploaded)
	r.Result = r.Uploaded
	return true
}

// DeleteFromDB deletes the object in r.Result from the db. We use the original DB object from
// API instead of r.DB as r.DB may have been edited with joins etc. and this breaks things.
func (r *request) DeleteFromDB() bool {
	log.WithFields(log.Fields{"item": r.Result}).Info("Deleting")
	r.api.db.Delete(r.Result)
	return true
}

// SerialiseResult Serialise r.Result to json and sends it back down the wire
func (r *request) SerialiseResult() bool {
	if r.Result == nil {
		log.Errorf("Serialise empty result")
		http.Error(r.W, "Not Found", 404)
		return false
	}
	err := json.NewEncoder(r.W).Encode(r.Result)
	if err != nil {
		log.Errorf("JSON Encode fail: %v", err)
		http.Error(r.W, `{"msg":"Failed to encode JSON"}`, 422)
		return false
	}
	return true
}
