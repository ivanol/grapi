package main

import (
	"log"
	"net/http"
	"os"

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
	os.Remove("./grapi-example.db")
	db, _ := gorm.Open("sqlite3", "./grapi-example.db")
	db.CreateTable(&Widget{})
	db.Create(&Widget{Name: "Test Widget"})

	// Initialise grapi
	api := grapi.New(grapi.Options{Db: db.Debug()})
	http.Handle("/api/", api)

	// Add index, get, post, patch and delete routes for widget
	api.AddDefaultRoutes(&Widget{})

	// Start Server
	log.Fatal(http.ListenAndServe("127.0.0.1:3000", nil))
}
