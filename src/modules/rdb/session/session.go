package session

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

type storage interface {
	all() int
	get(sid string) (*models.Session, error)
	insert(*models.Session) error
	del(sid string) error
	update(*models.Session) error
}

var (
	DefaultSession *Manager
)

func Init() {
	var err error
	DefaultSession, err = StartSession(&config.Config.HTTP.Session)
	if err != nil {
		panic(err)
	}
}

func Stop() {
	DefaultSession.StopGC()
}

func Start(w http.ResponseWriter, r *http.Request) (store *SessionStore, err error) {
	return DefaultSession.Start(w, r)
}

func Destroy(w http.ResponseWriter, r *http.Request) (string, error) {
	return DefaultSession.Destroy(w, r)
}

func GetSid(r *http.Request) (string, error) {
	return DefaultSession.GetSid(r)
}

func Get(sid string) (*SessionStore, error) {
	return DefaultSession.Get(sid)
}

func Exist(sid string) bool {
	return DefaultSession.Exist(sid)
}

func All() int {
	return DefaultSession.All()
}

func StartSession(cf *config.SessionSection, opts_ ...Option) (*Manager, error) {
	opts := &options{}

	for _, opt := range opts_ {
		opt.apply(opts)
	}

	if opts.ctx == nil {
		opts.ctx, opts.cancel = context.WithCancel(context.Background())
	}

	var storage storage
	var err error
	if cf.Storage == "mem" {
		storage, err = newMemStorage(cf, opts)
	} else {
		storage, err = newDbStorage(cf, opts)
	}

	if err != nil {
		return nil, err
	}

	return &Manager{
		storage: storage,
		options: opts,
		config:  cf,
	}, nil
}

type Manager struct {
	storage
	*options
	config *config.SessionSection
}

// SessionStart generate or read the session id from http request.
// if session id exists, return SessionStore with this id.
func (p *Manager) Start(w http.ResponseWriter, r *http.Request) (store *SessionStore, err error) {
	var sid string

	if sid, err = p.GetSid(r); err != nil {
		return
	}

	if sid != "" {
		if store, err := p.getSessionStore(sid, false); err == nil {
			return store, nil
		}
	}

	// Generate a new session
	sid = uuid.New().String()

	store, err = p.getSessionStore(sid, true)
	if err != nil {
		return nil, err
	}
	cookie := &http.Cookie{
		Name:     p.config.CookieName,
		Value:    url.QueryEscape(sid),
		Path:     "/",
		HttpOnly: p.config.HttpOnly,
		Domain:   p.config.Domain,
	}
	if p.config.CookieLifetime > 0 {
		cookie.MaxAge = int(p.config.CookieLifetime)
		cookie.Expires = time.Now().Add(time.Duration(p.config.CookieLifetime) * time.Second)
	}
	http.SetCookie(w, cookie)
	r.AddCookie(cookie)
	return
}

func (p *Manager) StopGC() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *Manager) Destroy(w http.ResponseWriter, r *http.Request) (string, error) {
	cookie, err := r.Cookie(p.config.CookieName)
	if err != nil || cookie.Value == "" {
		return "", fmt.Errorf("Have not login yet")
	}

	sid, _ := url.QueryUnescape(cookie.Value)
	logger.Debugf("session Destory sid %s", sid)
	p.del(sid)

	cookie = &http.Cookie{Name: p.config.CookieName,
		Path:     "/",
		HttpOnly: p.config.HttpOnly,
		Expires:  time.Now(),
		MaxAge:   -1}

	http.SetCookie(w, cookie)
	return sid, nil
}

func (p *Manager) Get(sid string) (*SessionStore, error) {
	return p.getSessionStore(sid, true)
}

func (p *Manager) Exist(sid string) bool {
	_, err := p.get(sid)
	return err == nil
}

// All count values in mysql session
func (p *Manager) All() int {
	return p.all()
}

func (p *Manager) GetSid(r *http.Request) (sid string, err error) {
	var cookie *http.Cookie

	cookie, err = r.Cookie(p.config.CookieName)
	if err != nil || cookie.Value == "" {
		return sid, nil
	}

	return url.QueryUnescape(cookie.Value)
}

func (p *Manager) getSessionStore(sid string, create bool) (*SessionStore, error) {
	sc, err := p.get(sid)
	if sc == nil && create {
		ts := time.Now().Unix()
		sc = &models.Session{
			Sid:       sid,
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		err = p.insert(sc)
	}
	if err != nil {
		return nil, err
	}
	return &SessionStore{manager: p, session: sc}, nil
}

// SessionStore mysql session store
type SessionStore struct {
	sync.RWMutex
	session *models.Session
	manager *Manager
}

// Set value in mysql session.
// it is temp value in map.
func (p *SessionStore) Set(k, v string) error {
	p.Lock()
	defer p.Unlock()
	switch k {
	case "username":
		p.session.Username = v
	case "remoteAddr":
		p.session.RemoteAddr = v
	case "accessToken":
		p.session.AccessToken = v
	default:
		fmt.Errorf("unsupported session field %s", k)
	}
	return nil
}

// Get value from mysql session
func (p *SessionStore) Get(k string) string {
	p.RLock()
	defer p.RUnlock()
	switch k {
	case "username":
		return p.session.Username
	case "remoteAddr":
		return p.session.RemoteAddr
	default:
		return ""
	}
}

func (p *SessionStore) CreatedAt() int64 {
	return p.session.CreatedAt
}

// Delete value in mysql session
func (p *SessionStore) Delete(k string) error {
	return p.Set(k, "")
}

// Reset clear all values in mysql session
func (p *SessionStore) Reset() error {
	p.Lock()
	defer p.Unlock()
	p.session.Username = ""
	p.session.RemoteAddr = ""
	return nil
}

// Sid get session id of this mysql session store
func (p *SessionStore) Sid() string {
	return p.session.Sid
}

func (p *SessionStore) Update(w http.ResponseWriter) error {
	p.session.UpdatedAt = time.Now().Unix()
	return p.manager.update(p.session)
}

const sessionKey = "context-session-key"

type contextKeyT string

var contextKey = contextKeyT("session")

/*
	ctx := NewContext(req.Context(), p)
	req = req.WithContext(ctx)
*/
// NewContext returns a copy of the parent context
// and associates it with an sessionStore.
func NewContext(ctx context.Context, s *SessionStore) context.Context {
	return context.WithValue(ctx, contextKey, s)
}

// FromContext returns the Auth bound to the context, if any.
func FromContext(ctx context.Context) (s *SessionStore, ok bool) {
	s, ok = ctx.Value(contextKey).(*SessionStore)
	return
}
