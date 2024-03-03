package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
)

var ExpectedAuthKeyHash [32]byte

type ContextKey string

const ContextKeyUnmarshalledJson = ContextKey("unmarshalledJson")
const ContextKeyWrappedRequest = ContextKey("wrappedRequest")

func (srv *HttpServer) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authKey := r.Header.Get("X-Auth-Key")

		if authKey == "" {
			http.Error(w, "Missing authorization", http.StatusUnauthorized)
			return
		}

		authKeyBase64, err := base64.StdEncoding.DecodeString(authKey)
		if err != nil {
			http.Error(w, "Invalid authorization", http.StatusUnauthorized)
			return
		}

		authKeyHash := sha256.Sum256(authKeyBase64)

		if !hmac.Equal(authKeyHash[:], ExpectedAuthKeyHash[:]) {
			http.Error(w, "Invalid authorization", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func recursiveValidate(v ModelWithValidation, ctx context.Context, srv *HttpServer) *Error {
	if validateErr := v.Validate(ctx, srv); validateErr != nil {
		return validateErr
	}
	return nil
	// FIXME
	// use reflection to find nested ModelWithValidation and validate them

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil
	}

	checkElem := func(elem reflect.Value) *Error {
		// if it's a pointer, we need to resolve it
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		if elem.Kind() != reflect.Struct {
			return nil
		}
		typeOfElem := elem.Type()
		whatToImplement := reflect.TypeOf((*ModelWithValidation)(nil)).Elem()
		// check if it implements ModelWithValidation
		// Validate() has a pointer receiver, so we need to check for pointer to the type
		if reflect.PointerTo(typeOfElem).Implements(whatToImplement) {
			if validateErr := recursiveValidate(elem.Addr().Interface().(ModelWithValidation), ctx, srv); validateErr != nil {
				return validateErr
			}
		}
		return nil
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if field.Kind() == reflect.Struct || field.Kind() == reflect.Ptr {
			if validateErr := checkElem(field); validateErr != nil {
				return validateErr
			}
		} else if field.Kind() == reflect.Slice {
			for j := 0; j < field.Len(); j++ {
				if validateErr := checkElem(field.Index(j)); validateErr != nil {
					return validateErr
				}
			}
		} else if field.Kind() == reflect.Map {
			for _, key := range field.MapKeys() {
				if validateErr := checkElem(field.MapIndex(key)); validateErr != nil {
					return validateErr
				}
			}

		}
	}

	return nil
}

func (srv *HttpServer) MustUnmarshalJsonMiddleware(next http.Handler, toGetter InterfaceGetter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := r.Context().Value(ContextKeyWrappedRequest).(*ReqWrapper)

		if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
		to := toGetter()
		if err = json.Unmarshal(body, to); err != nil {
			http.Error(w, "Error unmarshalling request body", http.StatusBadRequest)
			return
		}

		//if validateErr := to.Validate(r.Context(), srv); validateErr != nil {
		//	rw.WriteError(validateErr)
		//	return
		//}

		if validateErr := recursiveValidate(to, r.Context(), srv); validateErr != nil {
			rw.WriteError(validateErr)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), ContextKeyUnmarshalledJson, to))

		next.ServeHTTP(w, r)
	})
}

func (srv *HttpServer) WrapRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rw := &ReqWrapper{
			r:   r,
			w:   w,
			Srv: srv,
			Id:  randomString(8),
		}
		r = r.WithContext(context.WithValue(r.Context(), ContextKeyWrappedRequest, rw))
		rw.Debugf("Request: [%s] %s\n", r.Method, r.URL)
		w.Header().Set("X-Request-Id", rw.Id)

		next.ServeHTTP(w, r)
	})
}

func (srv *HttpServer) WrapAccessControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO
		w.Header().Set("Access-Control-Allow-Origin", "*")

		next.ServeHTTP(w, r)
	})
}

func init() {
	var err error

	ExpectedAuthKeyBase64Bytes, err := os.ReadFile("auth.key")

	if err != nil {
		if os.IsNotExist(err) {
			// Create a new auth key
			authKeyBytes := make([]byte, 64)
			_, _ = rand.Read(authKeyBytes)
			ExpectedAuthKeyBase64Bytes = []byte(base64.StdEncoding.EncodeToString(authKeyBytes))
			_ = os.WriteFile("auth.key", ExpectedAuthKeyBase64Bytes, 0600)

			fmt.Printf("Generated new auth key: %s\n", base64.StdEncoding.EncodeToString(authKeyBytes))
		} else {
			panic(err)
		}
	}

	ExpectedAuthKey, err := base64.StdEncoding.DecodeString(string(ExpectedAuthKeyBase64Bytes))
	ExpectedAuthKeyHash = sha256.Sum256(ExpectedAuthKey)

}
