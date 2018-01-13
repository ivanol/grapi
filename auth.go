package grapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"gopkg.in/dgrijalva/jwt-go.v2"

	log "github.com/Sirupsen/logrus"
)

// Objects that implement LoginModel can be used to login using the inbuilt authentication handlers.
type LoginModel interface {
	//CheckLoginDetails takes the body of the request (json deserialised to a map) and returns a login_id or error.
	CheckLoginDetails(json *map[string]interface{}, g *Grapi) (uint, error)

	//GetById returns the LoginModel by its id
	GetById(id uint, g *Grapi) (LoginModel, error)
}

// SetAuth sets the model used for logging in. Path will be added as a
// POST route to this model, with the LoginModel's AuthenticateJson method
// called in the handler to determine if authentication passes.
func (g *Grapi) SetAuth(model LoginModel, path string) {
	if g.options.JwtKey == "" {
		panic("Can't do authorisation safely unless you provide a random secret string as JwtKey parameter of api.New()")
	}
	g.options.LoginModel = model
	loginPath := g.options.UriPrefix + "/" + path
	log.Infof("Setting login path to %s", loginPath)

	g.router.Post(loginPath, g.loginHandler())
}

// loginHandler returns the handler for the path set in SetAuth. The handler
// expects to receive a json map which it will deserialise to map[string]interface{}
// and pass on to LoginModel.CheckLoginDetails
func (g *Grapi) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := httpBody(r)
		var f interface{}
		if err := json.Unmarshal(body, &f); err != nil {
			http.Error(w, `{"error":"Malformed JSON"}`, 422)
			log.Error("Receieved malformed JSON body")
			return
		}

		m := f.(map[string]interface{})
		user_id, err := g.options.LoginModel.CheckLoginDetails(&m, g)
		if err != nil {
			http.Error(w, `{"error":"Login failed"}`, 403)
			log.Errorf("Login Failed %v", err)
			return
		}
		log.Infof("Logged in user %v", user_id)
		token := getJWTToken(user_id, g.options.JwtKey)
		w.Write([]byte(`{"token":"` + token + `"}`))
	}
}

// defaultAuthenticator returns an Authenticator that looks for a jwt token in the http headers, authenticates it,
// and uses it to grab a LoginModel by id which it then stores in the request.
func (g *Grapi) defaultAuthenticator() Authenticator {
	return func(req ReqToAuthenticate) bool {
		r := req.GetRequest()
		w := req.GetResponseWriter()
		token, tokerr := jwt.ParseFromRequest(r, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				log.WithFields(log.Fields{"method": token.Header["alg"]}).Warn("JWT Auth: Unexpected signing method.")
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(req.Options().JwtKey), nil
		})
		if token != nil && token.Valid {
			guser, err := req.Options().LoginModel.GetById(uint(token.Claims["id"].(float64)), g)
			if err != nil {
				w.WriteHeader(401)
				fmt.Fprintf(w, "Unauthorized")
				log.WithFields(log.Fields{"id": token.Claims["id"]}).Warn("Cannot find logged in user")
				return false
			}
			user := guser.(LoginModel)
			req.SetLoginObject(user)
			return true
		} else {
			log.WithFields(log.Fields{"error": tokerr}).Warn("Auth: JWT token did not validate")
			http.Error(w, "Unauthorized", 401)
		}
		return false
	}
}

//Create a JWT token with id=id and expiring in 1 hour
func getJWTToken(id uint, key string) string {
	token := jwt.New(jwt.SigningMethodHS256)
	// Set some claims
	token.Claims["id"] = id
	token.Claims["exp"] = time.Now().Add(time.Hour * 1).Unix()
	log.WithFields(log.Fields{"expiry": token.Claims["exp"], "id": id}).Info("Signing token.")
	// Sign and get the complete encoded token as a string
	tokenString, err := token.SignedString([]byte(key))
	log.Printf("Token: %s, error %v", tokenString, err)
	return tokenString
}
