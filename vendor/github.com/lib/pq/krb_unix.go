// +build !windows

package pq

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/jcmturner/gokrb5/v8/client"
	"github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/spnego"
)

/*
 * UNIX Kerberos support, using jcmturner's pure-go
 * implementation
 */

// Implements the Gss interface
type gss struct {
	cli *client.Client
}

func NewGSS() (Gss, error) {
	g := &gss{}
	err := g.init()

	if err != nil {
		return nil, err
	}

	return g, nil
}

func (g *gss) init() error {
	cfgPath, ok := os.LookupEnv("KRB5_CONFIG")
	if !ok {
		cfgPath = "/etc/krb5.conf"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	u, err := user.Current()
	if err != nil {
		return err
	}

	ccpath := "/tmp/krb5cc_" + u.Uid

	ccname := os.Getenv("KRB5CCNAME")
	if strings.HasPrefix(ccname, "FILE:") {
		ccpath = strings.SplitN(ccname, ":", 2)[1]
	}

	ccache, err := credentials.LoadCCache(ccpath)
	if err != nil {
		return err
	}

	cl, err := client.NewFromCCache(ccache, cfg, client.DisablePAFXFAST(true))
	if err != nil {
		return err
	}

	cl.Login()

	g.cli = cl

	return nil
}

func (g *gss) GetInitToken(host string, service string) ([]byte, error) {

	// Resolve the hostname down to an 'A' record, if required (usually, it is)
	if g.cli.Config.LibDefaults.DNSCanonicalizeHostname {
		var err error
		host, err = canonicalizeHostname(host)
		if err != nil {
			return nil, err
		}
	}

	spn := service + "/" + host

	return g.GetInitTokenFromSpn(spn)
}

func (g *gss) GetInitTokenFromSpn(spn string) ([]byte, error) {
	s := spnego.SPNEGOClient(g.cli, spn)

	st, err := s.InitSecContext()
	if err != nil {
		return nil, fmt.Errorf("kerberos error (InitSecContext): %s", err.Error())
	}

	b, err := st.Marshal()
	if err != nil {
		return nil, fmt.Errorf("kerberos error (Marshaling token): %s", err.Error())
	}

	return b, nil
}

func (g *gss) Continue(inToken []byte) (done bool, outToken []byte, err error) {
	t := &spnego.SPNEGOToken{}
	err = t.Unmarshal(inToken)
	if err != nil {
		return true, nil, fmt.Errorf("kerberos error (Unmarshaling token): %s", err.Error())
	}

	state := t.NegTokenResp.State()
	if state != spnego.NegStateAcceptCompleted {
		return true, nil, fmt.Errorf("kerberos: expected state 'Completed' - got %d", state)
	}

	return true, nil, nil
}
