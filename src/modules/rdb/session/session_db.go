package session

import (
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/toolkits/pkg/logger"
)

func newDbStorage(cf *config.SessionSection, opts *options) (storage, error) {
	st := &dbStorage{config: cf}

	lifeTime := config.Config.HTTP.Session.CookieLifetime
	if lifeTime == 0 {
		if config.Config.Auth.ExtraMode.Enable {
			// cleanup by idle time worker
			lifeTime = 86400 * 10
		} else {
			lifeTime = 86400
		}
	}

	go func() {
		t := time.NewTicker(time.Second * time.Duration(cf.GcInterval))
		defer t.Stop()
		for {
			select {
			case <-opts.ctx.Done():
				return
			case <-t.C:
				err := models.SessionCleanupByCreatedAt(time.Now().Unix() - lifeTime)
				if err != nil {
					logger.Errorf("session gc err %s", err)
				}

			}
		}
	}()

	return st, nil
}

type dbStorage struct {
	config *config.SessionSection
}

func (p *dbStorage) all() int {
	n, err := models.SessionAll()
	if err != nil {
		logger.Errorf("sessionAll() err %s", err)
	}
	return int(n)
}

func (p *dbStorage) get(sid string) (*models.Session, error) {
	return models.SessionGet(sid)
}

func (p *dbStorage) insert(s *models.Session) error {
	return s.Save()

}

func (p *dbStorage) del(sid string) error {
	return models.SessionDelete(sid)
}

func (p *dbStorage) update(s *models.Session) error {
	return models.SessionUpdate(s)
}
