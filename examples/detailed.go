package main

import (
	"errors"
	"fmt"
	"hollo/grapi"
	"net"
	"net/http"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/zenazn/goji/graceful"
)

// User contains our table of users
type User struct {
	ID       uint   `gorm:"primary_key" json:"id"`
	Name     string `json:"name"`
	Password string `json:"-"` // `json:"-"` prevents password ever being serialized to json
	Admin    bool   `json:"admin"`
}

// Check the login details.
func (_ *User) CheckLoginDetails(j *map[string]interface{}, g *grapi.Grapi) (uint, error) {
	user := User{}
	fmt.Printf("Checking login details\n")
	if g.DB().Where("name = ? AND password = ?", (*j)["name"], (*j)["password"]).Find(&user).RecordNotFound() {
		fmt.Printf("Not found\n")
		return 0, errors.New("Not authenticated")
	} else {
		fmt.Printf("Found user %v\n", user)
		return user.ID, nil
	}
}

// Return user given an ID. This is the second function to make *User fulfill
// the LoginModel interface
func (_ *User) GetById(id uint, g *grapi.Grapi) (grapi.LoginModel, error) {
	user := User{}
	if g.DB().Where("id = ?", id).Find(&user).RecordNotFound() {
		return &user, errors.New("User not found")
	}
	return &user, nil
}

type PrivateWidget struct {
	ID     uint   `gorm:"primary_key" json:"id"`
	UserID uint   `json:"user_id"`
	Name   string `json:"name"`
}

type BelongsToUser interface {
	UserId() uint
}

func (pw *PrivateWidget) UserId() uint {
	return pw.UserID
}

func seedDb(db *gorm.DB) {
	db.DropTable(&User{})
	db.CreateTable(&User{})
	user1 := User{Name: "user1", Password: "user1"}
	user2 := User{Name: "user2", Password: "user2"}
	admin := User{Name: "admin", Password: "admin", Admin: true}
	db.Create(&user1)
	db.Create(&user2)
	db.Create(&admin)

	db.DropTable(&PrivateWidget{})
	db.CreateTable(&PrivateWidget{})
	db.Create(&PrivateWidget{Name: "User 1's Widget", UserID: user1.ID})
	db.Create(&PrivateWidget{Name: "User 2's Widget", UserID: user2.ID})
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "public/index.html")
}

func main() {
	// Create a DB with the test table and seed data
	db, _ := gorm.Open("sqlite3", "./grapi-example.db")
	seedDb(&db)

	// Create an API server. We need to supply JwtKey if we're doing authentication.
	// We pass db.Debug() instead of &db so you can see the sql queries in the log.
	a := grapi.New(grapi.Options{Db: db.Debug(), JwtKey: "SomethingLongAndDifficultToGuess"})
	http.Handle("/api/", a)
	http.HandleFunc("/", indexHandler)

	// Allow logging in with the User model at /api/login. Details will be checked by User.CheckLoginDetails()
	a.SetAuth(&User{}, "login")

	// Setup some useful RouteOptions that we will use for adding authenticated routs.

	// This one allows only authenticated users (ie. they've logged in at "/login" above).
	onlyAuthenticated := grapi.RouteOptions{UseDefaultAuth: true}

	// Only Allow Admin
	onlyAdmin := grapi.RouteOptions{
		UseDefaultAuth: true,
		// Add an authorize callback. This is a Martini handler, and can access the LoginModel
		// used for authentication. As we called API.SetAuth with &User{} this is guaranteed to
		// be a *User, so we can do a type assertion.
		Authorize: func(req grapi.ReqToAuthorize) bool {
			user := req.GetLoginObject().(*User)
			if !user.Admin {
				http.Error(req.GetResponseWriter(), `{"error":"You need to be admin to do that"}`, 403)
				return false
			}
			return true
		}}

	// This RouteOptions can be used for any table with a user_id field. If logged in as admin
	// it allows anything. If logged in as user it limits GETs to those of own user_id, and
	// delete to own user_id. It also prevents changing user ownership.
	onlyOwnUnlessAdmin := grapi.RouteOptions{
		UseDefaultAuth: true,
		Query: func(req grapi.ReqToLimit) bool {
			user := req.GetLoginObject().(*User)
			// Scope the requests database to only contain owned items. This prevents unauthorized
			// GET, DELETE, and PATCH requests, and limits the index to own items.
			if !user.Admin {
				req.SetDB(req.GetDB().Where("user_id = ?", user.ID))
			}
			return true
		},
		CheckUpload: func(req grapi.ReqULToCheck) bool {
			user := req.GetLoginObject().(*User)
			uploaded := req.GetUpload().(BelongsToUser)
			// For PATCH and POST routes we also need to check that the uploaded object has the correct user_id
			if !user.Admin && user.ID != uploaded.UserId() {
				http.Error(req.GetResponseWriter(), `{"error":"Only admin can change a user_id"}`, 403)
				return false
			}
			return true
		}}

	// Add the Default REST routes for User.
	// If two RouteOptions structures are provided the first is used for Read routes,
	// and the second for Write routes. If three are given then the third is used for
	// DELETE requests.
	a.AddDefaultRoutes(&User{}, onlyAuthenticated, onlyAdmin)

	// We want people to only see their own widgets, unless they are admin.
	a.AddDefaultRoutes(&PrivateWidget{}, onlyOwnUnlessAdmin)

	// We are going to make the widget list available to view by user at
	// /api/user/:user_id/private_widgets
	a.AddIndexRoute(&PrivateWidget{},
		&grapi.RouteOptions{
			Prefix: "/user/:user_id",
			Query: func(req grapi.ReqToLimit) bool {
				req.SetDB(req.GetDB().Where("user_id = ?", req.Param("user_id")))
				return true
			}})

	// Run the server.
	listener, err := net.Listen("tcp", "127.0.0.1:3000")
	if err != nil {
		panic(err.Error())
	}
	err = graceful.Serve(listener, http.DefaultServeMux)
	if err != nil {
		panic(err.Error())
	}

	graceful.Wait()
}
