package sso

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"

	"github.com/BurntSushi/toml"
)

// Init seeds these templates into sso_config and parses them back with
// log.Fatalln on failure, so a malformed one takes the whole center down on
// first start. Parse them here instead.
func TestSeedTemplatesParse(t *testing.T) {
	var ldapConfig ldapx.Config
	if err := toml.Unmarshal([]byte(LDAP), &ldapConfig); err != nil {
		t.Fatalf("parse LDAP template: %v", err)
	}
	var oidcConfig oidcx.Config
	if err := toml.Unmarshal([]byte(OIDC), &oidcConfig); err != nil {
		t.Fatalf("parse OIDC template: %v", err)
	}
	var casConfig cas.Config
	if err := toml.Unmarshal([]byte(CAS), &casConfig); err != nil {
		t.Fatalf("parse CAS template: %v", err)
	}
	var oauth2Config oauth2x.Config
	if err := toml.Unmarshal([]byte(OAuth2), &oauth2Config); err != nil {
		t.Fatalf("parse OAuth2 template: %v", err)
	}
}

// TestOAuth2DefaultTeams covers the whole path a DefaultTeams value takes: the
// TOML kept in sso_config -> oauth2x.Config -> the live SsoClient the login
// callback reads.
func TestOAuth2DefaultTeams(t *testing.T) {
	content := strings.Replace(OAuth2, "Enable = false", "Enable = true", 1)
	content = strings.Replace(content, "DefaultTeams = []", "DefaultTeams = [3, 7]", 1)

	var config oauth2x.Config
	if err := toml.Unmarshal([]byte(content), &config); err != nil {
		t.Fatalf("parse OAuth2 template: %v", err)
	}

	want := []int64{3, 7}
	if !reflect.DeepEqual(config.DefaultTeams, want) {
		t.Fatalf("config.DefaultTeams = %v, want %v", config.DefaultTeams, want)
	}

	client := oauth2x.New(config)
	if got := client.GetDefaultTeams(); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetDefaultTeams() = %v, want %v", got, want)
	}
}
