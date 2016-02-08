package grapi

import (
	"errors"
	"net/http"
	"testing"
)

type ErrorReadCloser struct {
}

//Implement io.Reader
func (e *ErrorReadCloser) Read(p []byte) (n int, err error) {
	return 0, errors.New("Failed as planned")
}

//Implement io.Closer
func (e *ErrorReadCloser) Close() error {
	return errors.New("We can't do this either")
}

func TestFailedHttpRead(t *testing.T) {
	r := http.Request{}
	r.Body = &ErrorReadCloser{}
	if len(httpBody(&r)) != 0 {
		t.Errorf("Received an httpBody from a non-existant Request")
	}
}

type StructWithID struct {
	ID uint
}

type StructWithoutID struct {
	id uint
}

func TestGetId(t *testing.T) {
	with := StructWithID{ID: 42}
	without := StructWithoutID{id: 42}
	id, err := getID(&with)
	if err != nil || id.(uint) != 42 {
		t.Errorf("Didn't get ID correctly")
	}
	id, err = getID(with)
	if err == nil {
		t.Errorf("Got ID when we shouldn't")
	}
	function := TestGetId
	id, err = getID(&function)
	if err == nil {
		t.Errorf("Got ID when we shouldn't")
	}
	id, err = getID(&without)
	if err == nil {
		t.Errorf("Got ID when we shouldn't")
	}
}
