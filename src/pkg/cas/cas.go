package cas

import (
	"bytes"
	"context"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/storage"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/cas"
	"github.com/toolkits/pkg/logger"
)

type Config struct {
	Enable           bool
	CASServer        string
	CASLoginCallback string
	CoverAttributes  bool
	Attributes       struct {
		Nickname string
		Phone    string
		Email    string
	}
	DefaultRoles []string
}

type ssoClient struct {
	config     Config
	attributes struct {
		username string
		nickname string
		phone    string
		email    string
	}
}

var (
	cli ssoClient
)

func Init(cf Config) {
	if !cf.Enable {
		return
	}
	cli = ssoClient{}
	cli.config = cf
	cli.attributes.username = "47"
	cli.attributes.nickname = cf.Attributes.Nickname
	cli.attributes.phone = cf.Attributes.Phone
	cli.attributes.email = cf.Attributes.Email
}

// Authorize return the cas authorize location and state
func Authorize(redirect string) (string, string, error) {
	state := uuid.New().String()
	ctx := context.Background()
	err := storage.Redis.Set(ctx, wrapStateKey(state), redirect, time.Duration(300*time.Second)).Err()
	if err != nil {
		return "", "", err
	}
	return cli.genRedirectURL(state), state, nil
}

func fetchRedirect(ctx context.Context, state string) (string, error) {
	return storage.Redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(ctx context.Context, state string) error {
	return storage.Redis.Del(ctx, wrapStateKey(state)).Err()
}

func wrapStateKey(key string) string {
	return "n9e_cas_" + key
}

func (cli *ssoClient) genRedirectURL(state string) string {
	var buf bytes.Buffer
	buf.WriteString(cli.config.CASServer + "login")
	v := url.Values{
		"service": {cli.config.CASLoginCallback},
	}
	if strings.Contains(cli.config.CASServer, "?") {
		buf.WriteByte('&')
	} else {
		buf.WriteByte('?')
	}
	buf.WriteString(v.Encode())
	return buf.String()
}

type CallbackOutput struct {
	Redirect    string `json:"redirect"`
	Msg         string `json:"msg"`
	AccessToken string `json:"accessToken"`
	Username    string `json:"username"`
	Nickname    string `json:"nickname"`
	Phone       string `yaml:"phone"`
	Email       string `yaml:"email"`
}

func ValidateServiceTicket(ctx context.Context, ticket, state string) (ret *CallbackOutput, err error) {
	casUrl, err := url.Parse(cli.config.CASServer)
	if err != nil {
		log.Fatal(err)
		return
	}
	serviceUrl, err := url.Parse(cli.config.CASLoginCallback)
	if err != nil {
		log.Fatal(err)
		return
	}
	resOptions := &cas.RestOptions{
		CasURL:     casUrl,
		ServiceURL: serviceUrl,
	}
	resCli := cas.NewRestClient(resOptions)
	authRet, err := resCli.ValidateServiceTicket(cas.ServiceTicket(ticket))
	if err != nil {
		logger.Errorf("Ticket Validating Failed: %s", err)
		return
	}
	ret = &CallbackOutput{}
	ret.Username = authRet.User
	ret.Nickname = cli.attributes.nickname
	ret.Email = cli.attributes.nickname
	ret.Phone = cli.attributes.phone
	ret.Redirect, err = fetchRedirect(ctx, state)
	if err != nil {
		logger.Debugf("get redirect err:%s state:%s", state, err)
	}
	err = deleteRedirect(ctx, state)
	if err != nil {
		logger.Debugf("delete redirect err:%s state:%s", state, err)
	}
	return
}
