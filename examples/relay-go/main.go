// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	ceremonyTTLSeconds = 120
	sessionTTLSeconds  = 3600
	headerTimeout      = 10 * time.Second
)

var badGateway = result{status: http.StatusBadGateway, body: []byte(`{"error":"bad_gateway"}`)}

type relay struct {
	parvusURL    string
	parvusSecret string
}

type exchange struct {
	context      context.Context
	body         requestBody
	slot         string
	sessionToken string
}

type requestBody struct {
	Username   string          `json:"username"`
	Credential json.RawMessage `json:"credential"`
}

type result struct {
	status  int
	body    []byte
	cookies []*http.Cookie
}

func main() {
	r := relay{parvusURL: os.Getenv("PARVUS_URL"), parvusSecret: os.Getenv("PARVUS_SECRET")}
	server := &http.Server{
		Addr:              os.Getenv("LISTEN"),
		Handler:           r.routes(),
		ReadHeaderTimeout: headerTimeout,
	}
	log.Fatal(server.ListenAndServe())
}

func (r relay) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.Dir("../browser")))
	mux.HandleFunc("POST /auth/registration/begin", serve(r.registrationBegin))
	mux.HandleFunc("POST /auth/registration/finish", serve(r.registrationFinish))
	mux.HandleFunc("POST /auth/login/begin", serve(r.loginBegin))
	mux.HandleFunc("POST /auth/login/finish", serve(r.loginFinish))
	mux.HandleFunc("GET /auth/me", serve(r.me))
	mux.HandleFunc("POST /auth/logout", serve(r.logout))
	return mux
}

func (r relay) registrationBegin(x exchange) result {
	return r.callParvus(x, "/v1/registration/begin", map[string]any{
		"slot":        x.slot,
		"user_id":     x.body.Username,
		"name":        x.body.Username,
		"ttl_seconds": ceremonyTTLSeconds,
	})
}

func (r relay) registrationFinish(x exchange) result {
	return r.callParvus(x, "/v1/registration/finish", map[string]any{
		"slot":       x.slot,
		"credential": x.body.Credential,
	})
}

func (r relay) loginBegin(x exchange) result {
	return r.callParvus(x, "/v1/login/begin", map[string]any{
		"slot":        x.slot,
		"ttl_seconds": ceremonyTTLSeconds,
	})
}

func (r relay) loginFinish(x exchange) result {
	res := r.callParvus(x, "/v1/login/finish", map[string]any{
		"slot":        x.slot,
		"credential":  x.body.Credential,
		"ttl_seconds": sessionTTLSeconds,
	})
	if res.status != http.StatusOK {
		return res
	}

	var session struct {
		UserID string `json:"user_id"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(res.body, &session); err != nil {
		return badGateway
	}
	return result{
		status:  http.StatusOK,
		body:    mustJSON(map[string]string{"user_id": session.UserID}),
		cookies: []*http.Cookie{cookie("session", session.Token, sessionTTLSeconds)},
	}
}

func (r relay) me(x exchange) result {
	res := r.callParvus(x, "/v1/introspect", map[string]any{"token": x.sessionToken})
	if res.status != http.StatusOK {
		return result{status: http.StatusUnauthorized, body: []byte(`{"error":"not_signed_in"}`)}
	}

	var session struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(res.body, &session); err != nil {
		return badGateway
	}
	return result{status: http.StatusOK, body: mustJSON(map[string]string{"user_id": session.UserID})}
}

func (r relay) logout(x exchange) result {
	r.callParvus(x, "/v1/logout", map[string]any{"token": x.sessionToken})
	return result{status: http.StatusNoContent, cookies: []*http.Cookie{cookie("session", "", -1)}}
}

func (r relay) callParvus(x exchange, path string, payload map[string]any) result {
	request, err := http.NewRequestWithContext(
		x.context, http.MethodPost, r.parvusURL+path, bytes.NewReader(mustJSON(payload)))
	if err != nil {
		return badGateway
	}
	request.Header.Set("Authorization", "Bearer "+r.parvusSecret)
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return badGateway
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return badGateway
	}
	return result{status: response.StatusCode, body: body}
}

func serve(handle func(exchange) result) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		x := describe(request)
		res := handle(x)

		http.SetCookie(w, cookie("slot", x.slot, ceremonyTTLSeconds))
		for _, c := range res.cookies {
			http.SetCookie(w, c)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.status)
		_, _ = w.Write(res.body)
	}
}

func describe(request *http.Request) exchange {
	x := exchange{context: request.Context(), slot: rand.Text()}
	if c, err := request.Cookie("slot"); err == nil {
		x.slot = c.Value
	}
	if c, err := request.Cookie("session"); err == nil {
		x.sessionToken = c.Value
	}
	x.body = bodyOrEmpty(request)
	return x
}

func bodyOrEmpty(request *http.Request) requestBody {
	var body requestBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return requestBody{}
	}
	return body
}

func cookie(name, value string, maxAgeSeconds int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
