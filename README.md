# grapi
This package uses [Goji](https://github.com/zenazn/goji),
[gorm] (https://github.com/jinzhu/gorm) and Golang to allow building RESTful
APIs with the minimum of time and boilerplate code. It was developed from 
an earlier piece of software [go-martini-api](https://github.com/ivanol/go-martini-api)
which used the now unsupported Martini to the same end.

## Simple Example
This example creates a webserver which serves (unauthenticated) REST
endpoints for Widget. These are:

| Verb    | URI            | Action                          |
|---------|----------------|-------
| GET     | /api/widgets   | Get full widget list
| GET     | /api/widgets/1 | Get widget with id==1
| POST    | /api/widgets   | Create a new widget
| PATCH   | /api/widgets/1 | Update widget with id==1
| DELETE  | /api/widgets/1 | Delete widget wit id==1

```go

package main

import (
	"net/http"

	"github.com/ivanol/grapi"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
)

type Widget struct {
	ID     uint   `gorm:"primary_key" json:"id"`
	Name   string `json:"name"`
	Public bool   `json:"public"`
}

func main() {
	// Create a DB with the test table and a seed widget
	db, _ := gorm.Open("sqlite3", "./grapi-example.db")
	db.CreateTable(&Widget{})
	db.Create(&Widget{Name: "Test Widget"})

	// Initialise grapi
	api := grapi.New(grapi.Options{Db: db.Debug()})
	http.Handle("/api/", api)

	// Add index, get, post, patch and delete routes for widget
	api.AddDefaultRoutes(&Widget{})

	// Start Server
	http.ListenAndServe("127.0.0.1:3000", nil)
}
```

## Customisation

Routes are added with `a.AddDefaultRoutes(modelPointer, grapi.RouteOptions{}...`. 
A RouteOptions structure can contain a series of callbacks which
will be called in turn, and can be used to authenticate, authorize, and otherwise
limit a route.

|            |GET|POST|PATCH|DELETE|
|------------|---|----|-----|------|
|Authenticate|Yes|Yes |Yes  |Yes   |
|Authorize   |Yes|Yes |Yes  |Yes   |
|Query       |Yes|    |Yes  |Yes   |
|CheckUpload |   |Yes |Yes  |      |
|EditResult  |Yes|Yes |Yes  |Yes   |

All of these Handlers have access to a different Request interface with which they
interact. The Query callback gives access to the gorm database object used
for queries for this request.  This can be used to scope the query used to
retrieve the existing DB object for GET, PATCH, and DELETE requests. Note
that `Create()`, `Save()`, and `Delete()` use the original DB object to actually
write to the database as otherwise they will not work if you add eg. a Join() to the
gorm object in the request. The actual query is made immediately after calling the Query
handler, and this should be used for any such scoping. eg:

```go
a.AddDefaultRoutes(&Widget{}, api.RouteOptions{
  Query: func(req *grapi.RequestToLimit) bool {
            req.DB = req.DB.Where("public = ?", true)
            return true
            } })
```

CheckUpload is called for POST and PATCH calls. `req.GetUpload()` will return
a pointer to the uploaded object, and this can be inspected, used to deny the request, or edited
before it is saved to the database. As an alternative (or addition) to CheckUpload, if
you implement the ValidateUpload function on your model pointer then it will fulfill
the NeedsValidation interface, and ValidateUpload will be called as part of the upload/
patch process.

EditResult is the last customisable handler and is called for all
calls. It has access to req.GetResult() and req.SetResult(), which is the
retrieved, added, edited, or deleted model depending on the call. For the
index it will be a slice of the model. This will by default be marshalled
to JSON and returned to the user. In EditResult it can be edited first,
or an entirely different result can be returned if wished.

## Authentication

The Authenticate handler method of RouteOptions can be used to carry
out a custom authentication strategy. If instead you wish to use the
builtin authentication you must you define an implemention of the
LoginModel interface. This can then be used with
`a.SetAuth('/login/, &MyLoginModel{})` to use jwt based authentication by simply setting
`RouteOptions{UseDefaultAuth: true}`.  The successfully logged in user
will be bound to all subsequent handlers as LoginModel.

## Detailed Example

A [detailed example](https://github.com/ivanol/grapi/blob/master/examples/detailed.go)
is in the examples folder. Run it with `go run detailed.go`, and then visit
http://localhost:3000/ to access it.
