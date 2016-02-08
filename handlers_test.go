package grapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Setup a route handler which returns a record of the handlers used!
type testRec struct {
	handlers string
}

func (t *testRec) record(handler string) {
	if len(t.handlers) > 0 {
		t.handlers += ":"
	}
	t.handlers += handler
}

func TestEmptyResult(t *testing.T) {
	api := getTestApi()
	api.AddDefaultRoutes(&PrivateWidget{},
		RouteOptions{
			UriModelName: ":test/testEmpty",
			EditResult: func(req ReqFinalResult) bool {
				if req.Param("test") == "yes" {
					req.SetResult(nil)
				} else {
					req.SetResult(req.Param("test"))
				}
				return true
			}})
	_ = testReq(t, "EmptyResult", "GET", "/api/yes/testEmpty", "", 404)
	body := testReq(t, "EmptyResult", "GET", "/api/workok/testEmpty", "", 200)
	if body != `"workok"` {
		t.Errorf("TestEmptyResult: Got a body of %v when we expected \"workok\"", body)
	}
}

func TestUnserialisableResult(t *testing.T) {
	api := getTestApi()
	api.AddDefaultRoutes(&PrivateWidget{},
		RouteOptions{
			UriModelName: ":test/testUnserialisable",
			EditResult: func(req ReqFinalResult) bool {
				if req.Param("test") == "yes" {
					req.SetResult(TestUnserialisableResult) // JSON should struggle to encode this function
				} else {
					req.SetResult(req.Param("test"))
				}
				return true
			}})
	_ = testReq(t, "UnserialisableResult", "GET", "/api/yes/testUnserialisable", "", 422)
	body := testReq(t, "UnserialisableResult", "GET", "/api/workok/testUnserialisable", "", 200)
	if body != `"workok"` {
		t.Errorf("TestUnserialisableResult: Got a body of %v when we expected \"workok\"", body)
	}
}

func TestQueryLimit(t *testing.T) {
	api := getTestApi()
	test_db.DropTable(&PrivateWidget{})
	test_db.CreateTable(&PrivateWidget{})

	test_db.Create(&PrivateWidget{ID: 1, UserID: 1, Name: "Widget 1"})
	test_db.Create(&PrivateWidget{ID: 2, UserID: 1, Name: "Widget 2"})
	test_db.Create(&PrivateWidget{ID: 3, UserID: 2, Name: "Widget 3"})

	api.AddIndexRoute(&PrivateWidget{},
		&RouteOptions{
			UriModelName: "user/:userid/private_widgets",
			Query: func(req ReqToLimit) bool {
				userid := req.Param("userid")
				req.SetDB(req.GetDB().Where("user_id = ?", userid))
				return true
			}})
	var pwList []PrivateWidget
	body := testReq(t, "User1Widgets", "GET", "/api/user/1/private_widgets", "", 200)
	json.Unmarshal([]byte(body), &pwList)
	if len(pwList) != 2 {
		t.Errorf("Received the wrong number of items when limiting: %d", len(pwList))
	}
	body = testReq(t, "User1Widgets", "GET", "/api/user/2/private_widgets", "", 200)
	json.Unmarshal([]byte(body), &pwList)
	if len(pwList) != 1 && pwList[0].ID != 3 {
		t.Errorf("Received the wrong item when limiting: %d", len(pwList))
	}
}

func TestCallbacks(t *testing.T) {
	api := getTestApi()
	api.AddDefaultRoutes(&PrivateWidget{},
		RouteOptions{
			UriModelName: "recordRoutes",
			Authenticate: func(req ReqToAuthenticate) bool {
				tr := testRec{req.Method()}
				tr.record("Authenticate")
				req.SetData(&tr)
				req.SetLoginObject("I am the Login Object")
				return true
			},
			Authorize: func(req ReqToAuthorize) bool {
				req.GetData().(*testRec).record("Authorize")
				loginObject := req.GetLoginObject().(string)
				if loginObject != "I am the Login Object" {
					t.Errorf("Didn't get the same login object back")
				}
				return true
			},
			Query: func(req ReqToLimit) bool { req.GetData().(*testRec).record("Query"); return true },
			CheckUpload: func(req ReqULToCheck) bool {
				req.GetData().(*testRec).record(fmt.Sprintf("CheckUpload(%s)", req.GetUpload().(*PrivateWidget).Name))
				return true
			},
			EditResult: func(req ReqFinalResult) bool {
				tr := req.GetData().(*testRec)
				if req.Method() == "PATCH" { //Lets not do this for GET as when we GET the index we'll have a []PrivateWidget not PrivateWidget
					tr.record(fmt.Sprintf("EditResult(%s)", req.GetResult().(*PrivateWidget).Name))
				} else {
					tr.record("EditResult")
				}
				req.SetResult(tr.handlers)
				return true
			}})
	// Note expected result is a marshalled json string - hence the `""` not ""
	testMethodHandlers(t, "TestCallbacks(GET)", "GET", `"GET:Authenticate:Authorize:Query:EditResult"`)
	testMethodHandlers(t, "TestCallbacks(POST)", "POST", `"POST:Authenticate:Authorize:CheckUpload(testname):EditResult"`)
	testMethodHandlers(t, "TestCallbacks(PATCH)", "PATCH", `"PATCH:Authenticate:Authorize:Query:CheckUpload(testname):EditResult(testname)"`)
	testMethodHandlers(t, "TestCallbacks(DELETE)", "DELETE", `"DELETE:Authenticate:Authorize:Query:EditResult"`)
}

// Test our we callback NeedsValidation interfaces appropriately.
func TestNeedsValidation(t *testing.T) {
	body := testReq(t, "PostItem", "POST", "/api/verified_widgets", `{"must_be_hello_world":"NewWidget"}`, 422)
	if body != `{"errors":{"must_be_hello_horld":"Is not equal to \"Hello World!!\""}}` {
		t.Errorf("Didn't receive correct error message for unverified widget: %s\n", body)
	}
	testReq(t, "PostItem", "POST", "/api/verified_widgets", `{"must_be_hello_world":"Hello World!!"}`, 200)
}

// helper function for TestCallbacks. Call the request, and check the expected
// series of callbacks is returned.
func testMethodHandlers(t *testing.T, name string, method string, expected string) {
	body := ""
	if method == "POST" || method == "PUT" || method == "PATCH" {
		body = `{"name":"testname"}`
	}
	uri := "/api/recordRoutes"
	if method == "DELETE" || method == "PATCH" {
		newWidget := PrivateWidget{Name: "ToDelete"}
		getTestApi().DB().Create(&newWidget)
		uri = fmt.Sprintf("%s/%d", uri, newWidget.ID)
	}
	handlers := testReq(t, name, method, uri, body, 200)
	if handlers != expected {
		t.Errorf("For %s expected handler list '%s', got '%s'", method, expected, handlers)
	} else {
		t.Logf("SUCCESS - Got handler list %s for %s", handlers, method)
	}
}

// itemHandlers returns a handler for returning a single item. Test with some requests
func TestItemHandlers(t *testing.T) {
	testReq(t, "GetItem", "GET", "/api/widgets/42", "", 404)
	body := testReq(t, "GetItem", "GET", "/api/widgets/2", "", 200)
	result := Widget{}
	json.Unmarshal([]byte(body), &result)
	if result.Name != "Widget 2" {
		t.Errorf("Failed to retrieve correct single item from the db: %v", result)
	}
}

// indexHandlers returns a handler for returning a list of items. Test with some requests
func TestIndexHandlers(t *testing.T) {
	body := testReq(t, "GetItem", "GET", "/api/widgets", "", 200)
	result := make([]Widget, 0)
	json.Unmarshal([]byte(body), &result)
	if len(result) != 3 {
		t.Errorf("Failed to retrieve correct numbers of widgets: %v", result)
	}
	for _, w := range result {
		if w.Name != fmt.Sprintf("Widget %d", w.ID) {
			t.Errorf("Didn't retrieve correct widget: %v", w)
		}
	}
}

// test post handlers
func TestPostHandlers(t *testing.T) {
	testReq(t, "PostItem(Malformed JSON)", "POST", "/api/widgets", `{"name""NewWidget"}`, 422)
	testReq(t, "PostItem(Existing Item ID)", "POST", "/api/widgets", `{"name":"NewWidget", "id":1}`, 422)
	body := testReq(t, "PostItem", "POST", "/api/widgets", `{"name":"NewWidget"}`, 200)
	newWidget := Widget{}
	checkWidget := Widget{}
	json.Unmarshal([]byte(body), &newWidget)
	getTestApi().DB().Where("id = ?", newWidget.ID).Find(&checkWidget)
	if newWidget.Name != "NewWidget" {
		t.Errorf("Didn't retrieve new object in POST request: %v", newWidget)
	}
	if checkWidget.Name != "NewWidget" {
		t.Errorf("Didn't save new object to DB in apparently successful POST request: %v", newWidget)
	}
	// Clear up
	getTestApi().DB().Delete(&checkWidget)
}

// patchHandlers returns a handler for patching an item. Test with some requests
func TestPatchHandlers(t *testing.T) {
	newWidget := Widget{Name: "ToEdit"}
	getTestApi().DB().Create(&newWidget)

	testReq(t, "EditItem(Doesn'tExist)", "PATCH", "/api/widgets/42", "", 404)
	testReq(t, "EditItem(MalformedJson)", "PATCH", fmt.Sprintf("/api/widgets/%v", newWidget.ID), `{"name:EditedName"}`, 422)
	testReq(t, "EditItem(EditID)", "PATCH", fmt.Sprintf("/api/widgets/%v", newWidget.ID), `{"id":0,"name":"EditedName"}`, 422)
	body := testReq(t, "EditItem", "PATCH", fmt.Sprintf("/api/widgets/%v", newWidget.ID), `{"name":"EditedName"}`, 200)
	checkWidget := Widget{}
	json.Unmarshal([]byte(body), &checkWidget)
	if checkWidget.Name != "EditedName" || checkWidget.ID != newWidget.ID {
		t.Errorf("Failed to return edited item on edit: %v != %v", checkWidget, newWidget)
	}
	if getTestApi().DB().Where("id = ?", newWidget.ID).Find(&checkWidget).RecordNotFound() {
		t.Errorf("The record we edited disappeared")
	} else {
		if checkWidget.Name != "EditedName" {
			t.Errorf("Failed to edit record in DB despite apparent success: %v != %v", newWidget, checkWidget)
		} else {
			t.Logf("PATCH widget succeeded")
		}
	}

}

// deleteHandlers returns a handler for deleting a single item. Test with some requests
func TestDeleteHandlers(t *testing.T) {
	newWidget := Widget{Name: "ToDelete"}
	getTestApi().DB().Create(&newWidget)

	testReq(t, "DeleteItem", "DELETE", "/api/widgets/42", "", 404)
	body := testReq(t, "DeleteItem", "DELETE", fmt.Sprintf("/api/widgets/%v", newWidget.ID), "", 200)
	checkWidget := Widget{}
	json.Unmarshal([]byte(body), &checkWidget)
	if checkWidget.Name != "ToDelete" {
		t.Errorf("Failed to return deleted item on delete: %v", checkWidget)
	}
	if getTestApi().DB().Where("id = ?", newWidget.ID).Find(&checkWidget).RecordNotFound() {
		t.Logf("SUCCESS - Record successfully deleted")
	} else {
		t.Errorf("Record not deleted when it should have been")
	}
}
