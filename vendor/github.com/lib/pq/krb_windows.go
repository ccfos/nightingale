// +build windows

package pq

import (
	"github.com/alexbrainman/sspi"
	"github.com/alexbrainman/sspi/negotiate"
)

type gss struct {
	creds *sspi.Credentials
	ctx   *negotiate.ClientContext
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
	creds, err := negotiate.AcquireCurrentUserCredentials()
	if err != nil {
		return err
	}

	g.creds = creds
	return nil
}

func (g *gss) GetInitToken(host string, service string) ([]byte, error) {

	host, err := canonicalizeHostname(host)
	if err != nil {
		return nil, err
	}

	spn := service + "/" + host

	return g.GetInitTokenFromSpn(spn)
}

func (g *gss) GetInitTokenFromSpn(spn string) ([]byte, error) {
	ctx, token, err := negotiate.NewClientContext(g.creds, spn)
	if err != nil {
		return nil, err
	}

	g.ctx = ctx

	return token, nil
}

func (g *gss) Continue(inToken []byte) (done bool, outToken []byte, err error) {
	return g.ctx.Update(inToken)
}
