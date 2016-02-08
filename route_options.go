package grapi

import (
	log "github.com/Sirupsen/logrus"
)

// RouteOptions can be applied to a single route or to a model. Pass them as
// one of the options to AddDefaultRoutes.
type RouteOptions struct {
	// A string to prefix all routes with. Eg. if the Model is called 'Employee' and
	// Prefix=="/department/:dept_id" then a list of all employees is found at
	// "/api/department/:dept_id/employees".
	// If you want to limit that list by :dept_id you need to define a callback
	// below
	Prefix string

	// By default the plural camel_case of the model name is used for the uri. eg.
	// if model is UserType the uri is /api/user_types. Override this here.
	UriModelName string

	// Handlers. If present these will be added in the following order. They will all
	// have access to a Request object containing the database handle, and can modify
	// this as required
	UseDefaultAuth bool          // If set to true then we'll use the internal Authenticator handler (ie. no need to set Authenticate)
	Authenticate   Authenticator // Use to set a custom authenticator.
	Authorize      Authorizor    // Use to authorize (if this can be done on route alone).
	Query          QueryLimiter  // Use to edit the db object (eg. add a Where or Preload)
	// Now the DB query will be carried out.
	// Now in a POST / PUT / PATCH request the uploaded object will be bound to req.Uploaded
	CheckUpload UploadChecker //POST/PUT/PATCH only. req.Uploaded will contain the upload object
	// The Request object should now contain a Result. This will be the object retrieved
	// from gorm, or the edited/deleted object. By default it will be marshalled and sent
	// back to the user. You can change that behaviour here.
	EditResult ResultEditor

	initialised bool
}

func (ro *RouteOptions) Initialise(g *Grapi) {
	// This could get expensive - just do it once.
	if ro.initialised {
		return
	}
	ro.initialised = true
	if !ro.UseDefaultAuth {
		return
	}
	if ro.Authenticate != nil {
		log.Panicf("Should set either RouteOptions.Authenticate or RouteOptions.UseDefaultAuth, but not both. " +
			"Your custom authenticator will now be overwritten.")
	}
	ro.Authenticate = g.defaultAuthenticator()
}
