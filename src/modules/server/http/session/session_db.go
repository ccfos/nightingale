package session

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/config"

	"github.com/toolkits/pkg/logger"
)

func newDbStorage(cf *config.SessionSection, opts *options) (storage, error) {
	st := &dbStorage{config: cf}

	lifeTime := config.Config.HTTP.Session.CookieLifetime
	if lifeTime == 0 {
		lifeTime = 86400
	}

	cleanup := func() {
		now := time.Now().Unix()
		err := models.SessionCleanupByUpdatedAt(now - lifeTime)
		if err != nil {
			logger.Errorf("session gc err %s", err)
		}

		n, err := models.DB["rdb"].Where("username='' and created_at < ?", now-lifeTime).Delete(new(models.Session))
		logger.Debugf("delete session %d lt created_at %d err %v", n, now-lifeTime, err)
	}

	go func() {
		cleanup()

		t := time.NewTicker(time.Second * time.Duration(cf.GcInterval))
		defer t.Stop()
		for {
			select {
			case <-opts.ctx.Done():
				return
			case <-t.C:
				cleanup()
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
