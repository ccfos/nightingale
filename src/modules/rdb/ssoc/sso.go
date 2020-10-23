package ssoc

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/util/cache"
)

type ssoClient struct {
	verifier        *oidc.IDTokenVerifier
	config          oauth2.Config
	apiKey          string
	cache           *cache.LRUExpireCache
	stateExpiresIn  time.Duration
	ssoAddr         string
	callbackAddr    string
	coverAttributes bool
	attributes      struct {
		username string
		dispname string
		phone    string
		email    string
		im       string
	}
}

var (
	cli ssoClient
)

func InitSSO() {
	cf := config.Config.SSO

	if !cf.Enable {
		return
	}

	cli.cache = cache.NewLRUExpireCache(1000)
	cli.ssoAddr = cf.SsoAddr
	cli.callbackAddr = cf.RedirectURL
	cli.coverAttributes = cf.CoverAttributes
	cli.attributes.username = "sub"
	cli.attributes.dispname = cf.Attributes.Dispname
	cli.attributes.phone = cf.Attributes.Phone
	cli.attributes.email = cf.Attributes.Email
	cli.attributes.im = cf.Attributes.Im
	provider, err := oidc.NewProvider(context.Background(), cf.SsoAddr)
	if err != nil {
		log.Fatal(err)
	}
	oidcConfig := &oidc.Config{
		ClientID: cf.ClientId,
	}

	cli.verifier = provider.Verifier(oidcConfig)
	cli.config = oauth2.Config{
		ClientID:     cf.ClientId,
		ClientSecret: cf.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cf.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	cli.apiKey = cf.ApiKey

	if cf.StateExpiresIn == 0 {
		cli.stateExpiresIn = time.Second * 60
	} else {
		cli.stateExpiresIn = time.Second * time.Duration(cf.StateExpiresIn)
	}
}

// Authorize return the sso authorize location with state
func Authorize(redirect string) string {
	state := uuid.New().String()
	cli.cache.Add(state, redirect, cli.stateExpiresIn)
	return cli.config.AuthCodeURL(state)
}

// LogoutLocation return logout location
func LogoutLocation(redirect string) string {
	redirect = fmt.Sprintf("%s?redirect=%s", cli.callbackAddr,
		url.QueryEscape(redirect))
	return fmt.Sprintf("%s/api/v1/account/logout?redirect=%s", cli.ssoAddr,
		url.QueryEscape(redirect))
}

// Callback 用 code 兑换 accessToken 以及 用户信息,
func Callback(code, state string) (string, *models.User, error) {
	s, ok := cli.cache.Get(state)
	if !ok {
		return "", nil, fmt.Errorf("invalid state %s", state)
	}
	cli.cache.Remove(state)

	redirect := s.(string)
	log.Printf("callback, get state %s redirect %s", state, redirect)

	u, err := exchangeUser(code)
	if err != nil {
		return "", nil, err
	}
	log.Printf("exchange user %v", u)

	user, err := models.UserGet("username=?", u.Username)
	if err != nil {
		return "", nil, err
	}

	if user == nil {
		user = u
		err = user.Save()
	} else if cli.coverAttributes {
		user.Email = u.Email
		user.Dispname = u.Dispname
		user.Phone = u.Phone
		user.Im = u.Im
		err = user.Update("email", "dispname", "phone", "im")
	}

	return redirect, user, err
}

func exchangeUser(code string) (*models.User, error) {
	ctx := context.Background()
	oauth2Token, err := cli.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("Failed to exchange token: %s", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("No id_token field in oauth2 token.")
	}
	idToken, err := cli.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("Failed to verify ID Token: %s", err)
	}

	data := map[string]interface{}{}
	if err := idToken.Claims(&data); err != nil {
		return nil, err
	}

	v := func(k string) string {
		if in := data[k]; in == nil {
			return ""
		} else {
			return in.(string)
		}
	}

	return &models.User{
		Username: v(cli.attributes.username),
		Dispname: v(cli.attributes.dispname),
		Phone:    v(cli.attributes.phone),
		Email:    v(cli.attributes.email),
		Im:       v(cli.attributes.im),
	}, nil
}

func CreateClient(w http.ResponseWriter, body io.ReadCloser) error {
	u := mkUrl("/api/v1/clients", nil)
	return req("POST", u, body, w)
}

func GetClients(w http.ResponseWriter, query url.Values) error {
	u := mkUrl("/api/v1/clients", query)
	return req("GET", u, nil, w)
}

func GetClient(w http.ResponseWriter, clientId string) error {
	u := mkUrl("/api/v1/clients/"+clientId, nil)
	return req("GET", u, nil, w)
}

func UpdateClient(w http.ResponseWriter, clientId string, body io.ReadCloser) error {
	u := mkUrl("/api/v1/clients/"+clientId, nil)
	return req("PUT", u, body, w)
}

func DeleteClient(w http.ResponseWriter, clientId string) error {
	u := mkUrl("/api/v1/clients/"+clientId, nil)
	return req("DELETE", u, nil, w)
}

func mkUrl(api string, query url.Values) string {
	if query == nil {
		return cli.ssoAddr + api
	}
	return cli.ssoAddr + api + "?" + query.Encode()
}

func req(method, u string, body io.ReadCloser, w http.ResponseWriter) error {
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", cli.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	if strings.HasPrefix(u, "https:") {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	return nil
}
