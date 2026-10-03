package server

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

func NewLocalAdminClient(userID int64) (*api.Client, func(), error) {
	user, err := model.GetUser(userID)
	if err != nil || user == nil || user.Role != model.RoleAdmin {
		return nil, nil, fmt.Errorf("administrator access required")
	}
	local := New()
	token, err := local.issueSession(user, time.Now().Add(12*time.Hour))
	if err != nil {
		return nil, nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	server := &http.Server{Handler: local}
	go func() { _ = server.Serve(listener) }()
	client, err := api.NewClient((&url.URL{Scheme: "http", Host: listener.Addr().String(), Path: "/api/v1"}).String())
	if err != nil {
		_ = server.Close()
		return nil, nil, err
	}
	client.SetToken(token)
	return client, func() { _ = server.Close() }, nil
}
