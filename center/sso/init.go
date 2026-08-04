package sso

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/dingtalk"
	"github.com/ccfos/nightingale/v6/pkg/feishu"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/BurntSushi/toml"
	"github.com/toolkits/pkg/logger"
)

type SsoClient struct {
	OIDC                 *oidcx.SsoClient
	LDAP                 *ldapx.SsoClient
	CAS                  *cas.SsoClient
	OAuth2               *oauth2x.SsoClient
	DingTalk             *dingtalk.SsoClient
	FeiShu               *feishu.SsoClient
	LastUpdateTime       int64
	configCache          *memsto.ConfigCache
	configLastUpdateTime int64
	// oidcNotReady 表示 OIDC 配置为开启但与 IdP 的对接还没成功（多为 IdP 暂时不可达），
	// 置位后即使 SSO 配置没有变更，reload 也会每个周期重试一次，IdP 恢复即自动补齐；
	// 定期 reload 协程和管理接口都会读写它，用 oidcMu 保护
	oidcNotReady bool
	oidcMu       sync.Mutex
}

// ReloadOIDC 是 OIDC 热更新的唯一入口：定期 reload 和管理员保存配置都走这里。它拿到
// 串行锁之后才去读库，不接受调用方传进来的配置快照——快照在排队等锁期间可能已经过期，
// 按快照发布会把管理员刚保存的新配置（甚至「关闭 OIDC」）覆盖回旧的，而 sso_config.update_at
// 只有秒级精度，两次更新落在同一秒时定期 reload 认不出配置变化，旧配置会长期生效。
// 以库为准还顺带保证「对接失败」一定被记下来，IdP 恢复后由定期 reload 自动补齐。
func (s *SsoClient) ReloadOIDC(ctx *ctx.Context) error {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()

	if s.OIDC == nil {
		return fmt.Errorf("oidc client is not initialized")
	}

	config, found, err := s.latestOIDCConfig(ctx)
	if err != nil {
		// 读不到库就不知道当前该信任谁：管理员可能刚把 OIDC 关掉或换掉不再受信的 IdP 且
		// 已经持久化成功，这时继续拿旧配置放行登录和 token 校验属于鉴权边界问题，先收紧；
		// 同时置位重试标记，等库恢复、读到最新配置后再重新启用
		s.OIDC.Reload(oidcx.Config{Enable: false})
		s.oidcNotReady = true
		return err
	}

	if !found {
		// 配置里已经没有 OIDC，不必再重试
		s.oidcNotReady = false
		return nil
	}

	logger.Info("reload oidc..")
	err = s.OIDC.Reload(config)
	s.oidcNotReady = err != nil
	return err
}

// latestOIDCConfig 读取库里最新的 OIDC 配置，第二个返回值表示配置是否存在
func (s *SsoClient) latestOIDCConfig(ctx *ctx.Context) (oidcx.Config, bool, error) {
	var config oidcx.Config

	configs, err := models.SsoConfigGets(ctx)
	if err != nil {
		return config, false, err
	}

	for _, cfg := range configs {
		if cfg.Name != "OIDC" {
			continue
		}

		content := tplx.ReplaceTemplateUseText(cfg.Name, cfg.Content, s.configCache.Get())
		err = toml.Unmarshal([]byte(content), &config)
		return config, true, err
	}

	return config, false, nil
}

func (s *SsoClient) oidcNeedsRetry() bool {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	return s.oidcNotReady
}

const LDAP = `
Enable = false
Host = 'ldap.example.org'
Port = 389
BaseDn = 'dc=example,dc=org'
BindUser = 'cn=manager,dc=example,dc=org'
BindPass = '*******'
SyncAddUsers = false
SyncDelUsers = false
# unit: s
SyncInterval = 86400
# openldap format e.g. (&(uid=%s))
# AD format e.g. (&(sAMAccountName=%s))
AuthFilter = '(&(uid=%s))'
UserFilter = '(&(uid=*))'
CoverAttributes = true
# Whether to overwrite roles with mapping (or DefaultRoles on miss) on each login. Requires CoverAttributes = true.
CoverRoles = false
TLS = false
StartTLS = true
DefaultRoles = ['Standard']

[Attributes]
Username = 'uid'
Nickname = 'cn'
Phone = 'mobile'
Email = 'mail'
`

const OAuth2 = `
Enable = false
DisplayName = 'Sign in with OAuth2'
RedirectURL = 'http://n9e.com/callback/oauth'
SsoAddr = 'https://sso.example.com/oauth2/authorize'
SsoLogoutAddr = 'https://sso.example.com/oauth2/authorize/session/end'
TokenAddr = 'https://sso.example.com/oauth2/token'
UserInfoAddr = 'https://api.example.com/api/v1/user/info'
TranTokenMethod = 'header'
ClientId = ''
ClientSecret = ''
CoverAttributes = true
DefaultRoles = ['Standard']
UserinfoIsArray = false
UserinfoPrefix = 'data'
Scopes = ['profile', 'email', 'phone']
# a2a/mcp OAuth2 Resource Server 的 access token 校验方式（HTTP.RSAuth.Provider="oauth2" 时生效）：
#   留空（默认）/ userinfo = 复用上面的 UserInfoAddr，靠 UserInfo 调用是否成功判定 token 有效，
#                            对接最省事（多数 OAuth2 server 都有 UserInfo）。注意：UserInfo 响应不含
#                            aud，本模式【不校验 audience】，同一 IdP 下任意有效 token 都会被接受。
#   introspect             = RFC 7662 内省，校验 token 的 aud，安全性更高；有安全要求时切到这个。
RSVerifyMethod = ''
# RFC 7662 token introspection 端点；RSVerifyMethod=introspect 时必填。IdP 的 introspection 响应
# 必须返回 aud（须包含 HTTP.RSAuth.Audience），否则拒绝。留空/userinfo 模式则复用上面的 UserInfoAddr。
IntrospectAddr = ''
# RS 校验结果按 token 哈希缓存的秒数（introspect 再以 token 自身 exp 封顶），0 表示不缓存。
IntrospectCacheSeconds = 60

[Attributes]
Username = 'sub'
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

const CAS = `
Enable = false
DisplayName = 'Sign in with CAS'
RedirectURL = 'http://n9e.com/callback/cas'
SsoAddr = 'https://cas.example.com/cas/'
SsoLogoutAddr = 'https://cas.example.com/cas/session/end'
# LoginPath = ''
CoverAttributes = true
DefaultRoles = ['Standard']

[Attributes]
Username = 'sub'
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

const OIDC = `
Enable = false
DisplayName = 'Sign in with OIDC'
RedirectURL = 'http://n9e.com/callback'
SsoAddr = 'http://sso.example.org'
SsoLogoutAddr = 'http://sso.example.org/session/end'
ClientId = ''
ClientSecret = ''
CoverAttributes = true
DefaultRoles = ['Standard']
Scopes = ['openid', 'profile', 'email', 'phone']

[Attributes]
Username = 'sub'
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

func Init(center cconf.Center, ctx *ctx.Context, configCache *memsto.ConfigCache) *SsoClient {
	ssoClient := new(SsoClient)
	m := make(map[string]string)
	m["LDAP"] = LDAP
	m["CAS"] = CAS
	m["OIDC"] = OIDC
	m["OAuth2"] = OAuth2

	for name, config := range m {
		count, err := models.SsoConfigCountByName(ctx, name)
		if err != nil {
			logger.Error(err)
			continue
		}

		if count > 0 {
			continue
		}

		ssoConfig := models.SsoConfig{
			Name:    name,
			Content: config,
		}

		err = ssoConfig.Create(ctx)
		if err != nil {
			log.Fatalln(err)
		}
	}
	if configCache == nil {
		log.Fatalln(fmt.Errorf("configCache is nil, sso initialization failed"))
	}
	ssoClient.configCache = configCache
	userVariableMap := configCache.Get()

	configs, err := models.SsoConfigGets(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	for _, cfg := range configs {
		cfg.Content = tplx.ReplaceTemplateUseText(cfg.Name, cfg.Content, userVariableMap)
		switch cfg.Name {
		case "LDAP":
			var config ldapx.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init ldap failed", err)
			}
			ssoClient.LDAP = ldapx.New(config)
		case "OIDC":
			var config oidcx.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init oidc failed:", err)
			}
			logger.Info("init oidc..")
			oidcClient, err := oidcx.New(config)
			if err != nil {
				// IdP 启动时不可达不该拖垮 center：客户端照常挂上（此时它处于未就绪状态，
				// 不会对外暴露登录入口），交给下面的定期 reload 持续重试
				logger.Errorf("init oidc failed: %v, will retry in background", err)
				ssoClient.oidcNotReady = true
			}
			ssoClient.OIDC = oidcClient
		case "CAS":
			var config cas.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init cas failed:", err)
			}
			ssoClient.CAS = cas.New(config)
		case "OAuth2":
			var config oauth2x.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init oauth2 failed:", err)
			}
			ssoClient.OAuth2 = oauth2x.New(config)
		case dingtalk.SsoTypeName:
			var config dingtalk.Config
			err := json.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalf("init %s failed: %s", dingtalk.SsoTypeName, err)
			}
			ssoClient.DingTalk = dingtalk.New(config)
		case feishu.SsoTypeName:
			var config feishu.Config
			err := json.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalf("init %s failed: %s", feishu.SsoTypeName, err)
			}
			ssoClient.FeiShu = feishu.New(config)
		}
	}

	// sso_config 表里没有 OIDC 记录时也保证客户端非空，这样 reload 协程和 HTTP handler
	// 都不必再改写这个指针，避免并发读写
	if ssoClient.OIDC == nil {
		ssoClient.OIDC = &oidcx.SsoClient{}
	}

	go ssoClient.SyncSsoUsers(ctx)
	go ssoClient.Reload(ctx)
	return ssoClient
}

// 定期更新sso配置
func (s *SsoClient) reload(ctx *ctx.Context) error {
	lastUpdateTime, err := models.SsoConfigLastUpdateTime(ctx)
	if err != nil {
		return err
	}

	lastCacheUpdateTime := s.configCache.GetLastUpdateTime()
	if lastUpdateTime == s.LastUpdateTime && lastCacheUpdateTime == s.configLastUpdateTime {
		// 配置没变时只补 OIDC：它还没对接成功就每个周期重试一次，IdP 恢复后无需重启 center。
		// 这里不能顺带跑完整个 reload —— LDAP 的重载会重置用户同步 Ticker，每 9 秒一次的话
		// 同步间隔稍长的 LDAP 就永远等不到触发了
		if s.oidcNeedsRetry() {
			return s.ReloadOIDC(ctx)
		}
		return nil
	}

	configs, err := models.SsoConfigGets(ctx)
	if err != nil {
		return err
	}
	userVariableMap := s.configCache.Get()
	ssoConfigMap := make(map[string]models.SsoConfig, 0)
	for _, cfg := range configs {
		ssoConfigMap[cfg.Name] = cfg
		cfg.Content = tplx.ReplaceTemplateUseText(cfg.Name, cfg.Content, userVariableMap)
		switch cfg.Name {
		case "LDAP":
			var config ldapx.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				logger.Warning("reload ldap failed", err)
				continue
			}
			s.LDAP.Reload(config)
		case "CAS":
			var config cas.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				logger.Warning("reload cas failed:", err)
				continue
			}
			s.CAS.Reload(config)
		case "OAuth2":
			var config oauth2x.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				logger.Warning("reload oauth2 failed:", err)
				continue
			}
			s.OAuth2.Reload(config)

		}
	}

	// OIDC 不走上面这份配置快照：ReloadOIDC 会在串行锁内重新读库，避免过期快照盖掉
	// 管理员刚保存的配置
	if err := s.ReloadOIDC(ctx); err != nil {
		logger.Error("reload oidc failed:", err)
	}

	if dingTalkConfig, ok := ssoConfigMap[dingtalk.SsoTypeName]; ok {
		var config dingtalk.Config
		err := json.Unmarshal([]byte(dingTalkConfig.Content), &config)
		if err != nil {
			logger.Warningf("reload %s failed: %s", dingtalk.SsoTypeName, err)
		} else {
			if s.DingTalk != nil {
				s.DingTalk.Reload(config)
			} else {
				s.DingTalk = dingtalk.New(config)
			}
		}
	} else {
		s.DingTalk = nil
	}

	if feiShuConfig, ok := ssoConfigMap[feishu.SsoTypeName]; ok {
		var config feishu.Config
		err := json.Unmarshal([]byte(feiShuConfig.Content), &config)
		if err != nil {
			logger.Warningf("reload %s failed: %s", feishu.SsoTypeName, err)
		} else {
			if s.FeiShu != nil {
				s.FeiShu.Reload(config)
			} else {
				s.FeiShu = feishu.New(config)
			}
		}
	} else {
		s.FeiShu = nil
	}

	s.LastUpdateTime = lastUpdateTime
	s.configLastUpdateTime = lastCacheUpdateTime
	return nil
}

func (s *SsoClient) Reload(ctx *ctx.Context) {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := s.reload(ctx); err != nil {
			logger.Warning("reload sso client err:", err)
		}
	}
}
